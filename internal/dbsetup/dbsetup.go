// Package dbsetup connects to a MySQL or MariaDB server (XAMPP, a standalone install, a hosted database) to test
// a connection or to create a database and a user for a game server, like txAdmin does.
//
// It never installs a database server and never stores the administrator login it is given: that is used once to
// create the database and a user whose rights stop at that database, and only the new user's login is kept (as the
// server's secret variables).
package dbsetup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Conn is a login to a MySQL/MariaDB server.
type Conn struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)
var hostRe = regexp.MustCompile(`^[A-Za-z0-9.\-:\[\]]{1,253}$`)

// Validate checks the fields before anything is sent to the network.
func (c *Conn) Validate(needDatabase bool) error {
	c.Host, c.User = strings.TrimSpace(c.Host), strings.TrimSpace(c.User)
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 3306
	}
	switch {
	case !hostRe.MatchString(c.Host):
		return errors.New("enter the database host, for example 127.0.0.1")
	case c.Port < 1 || c.Port > 65535:
		return errors.New("the port must be between 1 and 65535")
	case c.User == "" || len(c.User) > 80 || strings.ContainsAny(c.User, "'\"`\\\x00"):
		return errors.New("enter the database user")
	case len(c.Password) > 200:
		return errors.New("the password is too long")
	case needDatabase && !nameRe.MatchString(c.Database):
		return errors.New("the database name may use letters, numbers and underscores (up to 32)")
	}
	return nil
}

func (c Conn) open(withDB bool) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net = c.User, c.Password, "tcp"
	cfg.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	if withDB {
		cfg.DBName = c.Database
	}
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 8*time.Second, 15*time.Second, 15*time.Second
	cfg.AllowNativePasswords = true
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	return db, nil
}

// Test connects with c and checks that the database can be used.
func Test(ctx context.Context, c Conn) error {
	db, err := c.open(true)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return explain(err)
	}
	return nil
}

// Create uses an administrator login to create the database (utf8mb4) and a user with a new random password and
// rights on that database only. If the user already exists it gets the new password. The returned Conn is the new
// user's login.
func Create(ctx context.Context, admin Conn, database, user string) (Conn, error) {
	if !nameRe.MatchString(database) {
		return Conn{}, errors.New("the database name may use letters, numbers and underscores (up to 32)")
	}
	if !nameRe.MatchString(user) {
		return Conn{}, errors.New("the user name may use letters, numbers and underscores (up to 32)")
	}
	db, err := admin.open(false)
	if err != nil {
		return Conn{}, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return Conn{}, explain(err)
	}
	password := randomPassword()
	// Identifiers and the password are checked above (letters, numbers, underscores), so quoting them is safe;
	// MySQL doesn't take placeholders in these statements.
	hosts := []string{"%"}
	if isLocal(admin.Host) {
		hosts = []string{"localhost", "127.0.0.1", "::1"}
	}
	stmts := []string{"CREATE DATABASE IF NOT EXISTS `" + database + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"}
	for _, h := range hosts {
		stmts = append(stmts,
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'", user, h, password),
			fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'", user, h, password),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s'", database, user, h),
		)
	}
	stmts = append(stmts, "FLUSH PRIVILEGES")
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return Conn{}, explain(err)
		}
	}
	out := Conn{Host: admin.Host, Port: admin.Port, User: user, Password: password, Database: database}
	if err := Test(ctx, out); err != nil {
		return Conn{}, fmt.Errorf("the database was created, but the new user can't sign in: %w", err)
	}
	return out, nil
}

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func randomPassword() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// explain turns driver errors into something a server owner can act on.
func explain(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1045:
			return errors.New("the user name or password is wrong")
		case 1044, 1142, 1227:
			return errors.New("this user isn't allowed to do that; use an administrator login such as root")
		case 1049:
			return errors.New("that database doesn't exist")
		}
		return fmt.Errorf("the database answered: %s", me.Message)
	}
	var ne net.Error
	if errors.As(err, &ne) || strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "actively refused") {
		return errors.New("can't reach a database server there. Is MySQL/MariaDB (or XAMPP's MySQL) running, and is the host and port right?")
	}
	return err
}
