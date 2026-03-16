package dissect

import "encoding/json"

func operatorSeasonKey(gameVersion string) string {
	switch {
	case hasSeasonPrefix(gameVersion, "Y11S2"):
		return "Y11S2"
	case hasSeasonPrefix(gameVersion, "Y11S3"):
		return "Y11S3"
	case hasSeasonPrefix(gameVersion, "Y11S4"):
		return "Y11S4"
	default:
		return ""
	}
}

func hasSeasonPrefix(gameVersion string, season string) bool {
	if len(gameVersion) < len(season) {
		return false
	}
	return gameVersion[:len(season)] == season
}

func (i Operator) fallbackNameForGameVersion(gameVersion string) string {
	switch operatorSeasonKey(gameVersion) {
	case "Y11S2":
		return Dokkaebi.String()
	case "Y11S3":
		return "Y11S3NewDefender"
	case "Y11S4":
		return "Y11S4UnknownOperator"
	default:
		return i.String()
	}
}

func (i Operator) NameForGameVersion(gameVersion string) string {
	if name, ok := _Operator_map[i]; ok {
		return name
	}
	return i.fallbackNameForGameVersion(gameVersion)
}

func (i Operator) RoleForGameVersion(gameVersion string) (TeamRole, bool) {
	if role, ok := _operatorRoles[i]; ok {
		return role, true
	}
	switch operatorSeasonKey(gameVersion) {
	case "Y11S2":
		return Attack, true
	case "Y11S3":
		return Defense, true
	default:
		return "", false
	}
}

func (p Player) MarshalJSON() ([]byte, error) {
	type playerJSON struct {
		ID           uint64             `json:"id,omitempty"`
		ProfileID    string             `json:"profileID,omitempty"`
		Username     string             `json:"username"`
		TeamIndex    int                `json:"teamIndex"`
		Operator     stringerIntMarshal `json:"operator"`
		HeroName     int                `json:"heroName,omitempty"`
		Alliance     int                `json:"alliance"`
		RoleImage    int                `json:"roleImage,omitempty"`
		RoleName     string             `json:"roleName,omitempty"`
		RolePortrait int                `json:"rolePortrait,omitempty"`
		Spawn        string             `json:"spawn,omitempty"`
	}
	return json.Marshal(playerJSON{
		ID:        p.ID,
		ProfileID: p.ProfileID,
		Username:  p.Username,
		TeamIndex: p.TeamIndex,
		Operator: stringerIntMarshal{
			Name: p.Operator.NameForGameVersion(p.gameVersion),
			ID:   int(p.Operator),
		},
		HeroName:     p.HeroName,
		Alliance:     p.Alliance,
		RoleImage:    p.RoleImage,
		RoleName:     p.RoleName,
		RolePortrait: p.RolePortrait,
		Spawn:        p.Spawn,
	})
}
