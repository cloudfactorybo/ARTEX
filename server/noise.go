package server

import (
	"log"
	"net/http"
	"os"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/egress"
)

// startupAgentLanguage is the agents' output language at startup: the stored
// agent_language setting when valid, else ARTEX_AGENT_LANGUAGE, else Spanish.
func startupAgentLanguage(pg *db.DB) string {
	v, ok, err := pg.GetSetting(settingAgentLanguage)
	if err != nil {
		log.Printf("[agent] read %s setting: %v; using ARTEX_AGENT_LANGUAGE", settingAgentLanguage, err)
	}
	if ok {
		if code, valid := agent.ParseAgentLanguage(v); valid {
			return code
		}
		log.Printf("[agent] stored %s=%q is not es, en or auto; using ARTEX_AGENT_LANGUAGE", settingAgentLanguage, v)
	}
	if code, valid := agent.ParseAgentLanguage(os.Getenv("ARTEX_AGENT_LANGUAGE")); valid {
		return code
	}
	return agent.DefaultAgentLanguage
}

// startupNoiseProfile is the noise profile to apply at startup: the stored
// noise_profile setting when there is a valid one, else ARTEX_NOISE_PROFILE.
func startupNoiseProfile(pg *db.DB) egress.Profile {
	v, ok, err := pg.GetSetting(settingNoiseProfile)
	if err != nil {
		log.Printf("[egress] read %s setting: %v; using ARTEX_NOISE_PROFILE", settingNoiseProfile, err)
	}
	if ok {
		if p, valid := egress.ProfileByName(v); valid {
			return p
		}
		log.Printf("[egress] stored %s=%q is not off, quiet, balanced or aggressive; using ARTEX_NOISE_PROFILE", settingNoiseProfile, v)
	}
	return egress.ProfileFromEnv()
}

// noiseStatus returns the current noise profile and the per-host counters of
// its governor, read without starting a new minute.
func (s *Server) noiseStatus(w http.ResponseWriter, r *http.Request) {
	g := egress.Current()
	writeJSON(w, 200, map[string]any{"profile": g.Profile().Name, "hosts": g.Snapshot()})
}
