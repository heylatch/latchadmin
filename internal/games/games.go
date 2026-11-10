// Package games lists the MafiaHub games LatchAdmin can install. Adding a game is one entry here, as long as its
// release zip has the usual layout: <Name>-<version>/{server, server-linux, client, resources?}.
package games

import "runtime"

type Game struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Slug        string   `json:"slug"` // api.mafiahub.dev/mods/<slug>
	Exe         string   `json:"exe"`  // server binary name without extension
	OldExes     []string `json:"-"`    // names used by older releases, still recognised when importing
	DefaultPort int      `json:"defaultPort"`
}

var All = []Game{
	{ID: "arcane-mp", Label: "Arcane MP", Slug: "hogwarts-mp", Exe: "ArcaneMPServer", OldExes: []string{"HogwartsMPServer"}, DefaultPort: 27015},
	{ID: "kingdoms-connected", Label: "Kingdoms Connected", Slug: "kcdc", Exe: "KCDCServer", DefaultPort: 27015},
	{ID: "mafia2-online", Label: "Mafia 2 Online", Slug: "m2o", Exe: "M2OServer", DefaultPort: 27015},
	// No public release on api.mafiahub.dev yet, so their binary names aren't known: install unlocks when a release
	// appears; until then an existing server is added by picking its binary.
	{ID: "fallout-mp", Label: "Fallout MP", Slug: "fmp", DefaultPort: 27015},
	{ID: "mafia-mp", Label: "Mafia MP", Slug: "mafia-mp", DefaultPort: 27015},
	{ID: "vanta-mp", Label: "Vanta MP", Slug: "vanta", DefaultPort: 27015},
}

func ByID(id string) (Game, bool) {
	for _, g := range All {
		if g.ID == id {
			return g, true
		}
	}
	return Game{}, false
}

// ExeNames are the binary file names to look for on this OS, current name first.
func (g Game) ExeNames() []string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	var names []string
	if g.Exe != "" {
		names = append(names, g.Exe+ext)
	}
	for _, o := range g.OldExes {
		names = append(names, o+ext)
	}
	return names
}

// ServerFolder is the folder inside the release zip with this OS's server runtime.
func ServerFolder() string {
	if runtime.GOOS == "windows" {
		return "server"
	}
	return "server-linux"
}
