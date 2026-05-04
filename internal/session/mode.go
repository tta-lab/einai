package session

// Mode identifies the ask operating mode.
type Mode string

const (
	ModeProject Mode = "project"
	ModeRepo    Mode = "repo"
	ModeURL     Mode = "url"
	ModeWeb     Mode = "web"
	ModeGeneral Mode = "general"
)

// Valid reports whether m is a known ask mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeProject, ModeRepo, ModeURL, ModeWeb, ModeGeneral:
		return true
	}
	return false
}

// ModeParams holds mode-specific parameters for prompt building.
type ModeParams struct {
	WorkingDir    string
	ProjectPath   string
	RepoLocalPath string
	RawURL        string
	Question      string
}
