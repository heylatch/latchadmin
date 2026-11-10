// Package perms lists what a panel user may do. Each user has a set of permissions; "all" grants everything
// (the panel always keeps at least one such user).
package perms

const All = "all"

type Group struct {
	ID    string   `json:"id"`
	Perms []string `json:"perms"`
}

// Groups is the full list, grouped for the Users page.
var Groups = []Group{
	{ID: "servers", Perms: []string{"server.control", "server.settings", "server.add", "server.remove"}},
	{ID: "console", Perms: []string{"console.view", "console.write"}},
	{ID: "resources", Perms: []string{"resources.manage"}},
	{ID: "config", Perms: []string{"config.view", "config.edit"}},
	{ID: "updates", Perms: []string{"updates.game", "updates.scripts"}},
	{ID: "panel", Perms: []string{"panel.users", "panel.settings", "panel.update", "panel.latch", "panel.activity"}},
}

// implied: having the key also grants the value.
var implied = map[string]string{"console.write": "console.view", "config.edit": "config.view"}

// Valid reports whether p is a known permission.
func Valid(p string) bool {
	if p == All {
		return true
	}
	for _, g := range Groups {
		for _, x := range g.Perms {
			if x == p {
				return true
			}
		}
	}
	return false
}

// Has reports whether a permission set includes p.
func Has(set []string, p string) bool {
	for _, x := range set {
		if x == All || x == p || implied[x] == p {
			return true
		}
	}
	return false
}

// Clean removes unknown and duplicate entries; "all" alone replaces everything else.
func Clean(set []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range set {
		if p == All {
			return []string{All}
		}
		if Valid(p) && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// FromRole converts the roles of LatchAdmin 0.1 to permissions.
func FromRole(role string) []string {
	switch role {
	case "owner":
		return []string{All}
	case "admin":
		return []string{"server.control", "server.settings", "console.write", "resources.manage", "config.edit", "updates.game", "updates.scripts", "panel.activity"}
	default:
		return []string{"console.view", "config.view"}
	}
}
