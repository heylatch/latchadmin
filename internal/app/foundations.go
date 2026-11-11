package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/dbsetup"
	"github.com/heylatch/latchadmin/internal/foundations"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

// ---------- Database ----------

// DatabaseSetup is what the panel asks for: create a database with an administrator login, or use an existing one.
type DatabaseSetup struct {
	Mode     string       `json:"mode"` // "create" or "existing"
	Conn     dbsetup.Conn `json:"conn"`
	Database string       `json:"database"` // create: name of the new database
	User     string       `json:"user"`     // create: name of the new user
}

// PrepareDatabase creates or checks a database and returns the login the game server will use.
func (a *App) PrepareDatabase(ctx context.Context, in DatabaseSetup) (dbsetup.Conn, error) {
	switch in.Mode {
	case "create":
		if err := in.Conn.Validate(false); err != nil {
			return dbsetup.Conn{}, err
		}
		return dbsetup.Create(ctx, in.Conn, orDefault(in.Database, "arcanemp"), orDefault(in.User, "arcanemp"))
	case "existing":
		if err := in.Conn.Validate(true); err != nil {
			return dbsetup.Conn{}, err
		}
		return in.Conn, dbsetup.Test(ctx, in.Conn)
	}
	return dbsetup.Conn{}, errors.New("choose how to set up the database")
}

// Foundations reads its database from these variables (they override data/hmp-mysql.json).
const (
	envHost = "HMP_MYSQL_HOST"
	envPort = "HMP_MYSQL_PORT"
	envUser = "HMP_MYSQL_USER"
	envPass = "HMP_MYSQL_PASSWORD"
	envDB   = "HMP_MYSQL_DATABASE"
)

// databaseEnv sets the HMP_MYSQL_* variables on a server (the password as a secret).
func databaseEnv(env []store.EnvVar, c dbsetup.Conn) ([]store.EnvVar, error) {
	sealed, err := secret.Seal(c.Password)
	if err != nil {
		return nil, err
	}
	set := map[string]store.EnvVar{
		envHost: {Key: envHost, Value: c.Host},
		envPort: {Key: envPort, Value: strconv.Itoa(c.Port)},
		envUser: {Key: envUser, Value: c.User},
		envPass: {Key: envPass, Value: sealed, Secret: true},
		envDB:   {Key: envDB, Value: c.Database},
	}
	out := env[:0:0]
	for _, e := range env {
		if _, ok := set[e.Key]; !ok && e.Key != "HMP_MYSQL_URL" { // a URL would win over the parts
			out = append(out, e)
		}
	}
	for _, k := range []string{envHost, envPort, envUser, envPass, envDB} {
		out = append(out, set[k])
	}
	return out, nil
}

// SetDatabase stores a database login on a server. It applies from the server's next start.
func (a *App) SetDatabase(id string, c dbsetup.Conn, user string) error {
	err := a.Store.UpdateServer(id, func(sv *store.Server) error {
		env, err := databaseEnv(sv.Env, c)
		sv.Env = env
		return err
	})
	if err == nil {
		sv, _ := a.Store.Server(id)
		a.Store.Audit(user, "server.database", sv.Name, map[string]string{"host": c.Host, "database": c.Database, "user": c.User})
	}
	return err
}

// DatabaseInfo is the database a server uses, without the password.
type DatabaseInfo struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Database string `json:"database"`
}

func databaseInfo(sv store.Server) *DatabaseInfo {
	var d DatabaseInfo
	for _, e := range sv.Env {
		switch e.Key {
		case envHost:
			d.Host = e.Value
		case envPort:
			d.Port = e.Value
		case envUser:
			d.User = e.Value
		case envDB:
			d.Database = e.Value
		}
	}
	if d.Host == "" || d.Database == "" {
		return nil
	}
	return &d
}

// ---------- Foundations ----------

type foundationsCache struct {
	mu  sync.Mutex
	rel foundations.Release
	err error
	at  time.Time
}

var fcache foundationsCache

// latestFoundations asks GitHub at most every 30 minutes (its API allows 60 calls an hour without a key).
func latestFoundations(ctx context.Context) (foundations.Release, error) {
	fcache.mu.Lock()
	defer fcache.mu.Unlock()
	if time.Since(fcache.at) < 30*time.Minute && (fcache.err == nil || time.Since(fcache.at) < 2*time.Minute) {
		return fcache.rel, fcache.err
	}
	fcache.rel, fcache.err = foundations.Latest(ctx)
	fcache.at = time.Now()
	return fcache.rel, fcache.err
}

// FoundationsStatus is what the server's Updates tab shows.
type FoundationsStatus struct {
	Installed string        `json:"installed"`
	Latest    string        `json:"latest"`
	URL       string        `json:"url"`
	Error     string        `json:"error,omitempty"`
	Conflicts []string      `json:"conflicts"`
	Database  *DatabaseInfo `json:"database"`
}

func (a *App) FoundationsStatus(ctx context.Context, id string) (FoundationsStatus, error) {
	sv, ok := a.Store.Server(id)
	if !ok {
		return FoundationsStatus{}, store.ErrNotFound
	}
	st := FoundationsStatus{Installed: foundations.Installed(sv.Dir), Conflicts: foundations.Conflicts(sv.Dir), Database: databaseInfo(sv)}
	if st.Conflicts == nil {
		st.Conflicts = []string{}
	}
	if rel, err := latestFoundations(ctx); err != nil {
		st.Error = err.Error()
	} else {
		st.Latest, st.URL = rel.Version, rel.URL
	}
	return st, nil
}

// InstallFoundations installs or updates Foundations on an Arcane MP server. A running server is stopped first
// and started again afterwards (all thirty resources change together).
func (a *App) InstallFoundations(id, user string) (*Job, error) {
	sv, ok := a.Store.Server(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	if sv.Game != foundations.Game {
		return nil, errors.New("Foundations is for Arcane MP servers")
	}
	return a.Jobs.Start("foundations", id, func(ctx context.Context, j *Job) (any, error) {
		j.Set("check", 0, 0)
		rel, err := latestFoundations(ctx)
		if err != nil {
			return nil, err
		}
		p := a.Proc(id)
		wasUp := p.Status().State == supervisor.Running || p.Status().State == supervisor.Starting
		if wasUp {
			j.Set("stop", 0, 0)
			_ = p.Stop()
		}
		if err := p.SetUpdating(true); err != nil {
			return nil, err
		}
		res, err := foundations.Install(ctx, sv.Dir, rel, func(stage string) { j.Set(stage, 0, 0) })
		_ = p.SetUpdating(false)
		if err != nil {
			p.Panelf("Foundations %s failed: %v", rel.Version, err)
		} else {
			p.Panelf("Foundations %s installed (%d resources). New example configs: %s", res.Version, res.Resources, strings.Join(res.Configs, ", "))
			a.Store.Audit(user, "foundations.install", sv.Name, map[string]string{"from": res.Previous, "to": res.Version})
		}
		if wasUp {
			if serr := p.Start(); serr != nil && err == nil {
				err = serr
			}
		}
		return res, err
	})
}

func orDefault(v, def string) string {
	if v = strings.TrimSpace(v); v == "" {
		return def
	}
	return v
}
