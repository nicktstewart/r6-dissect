package dissect

import (
	"encoding/json"
	"strings"
	"unicode"
)

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

func operatorNameKey(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case 'Ä', 'Á', 'À', 'Â', 'Ã', 'Å', 'Ā', 'Ă', 'Ą':
			r = 'A'
		case 'ä', 'á', 'à', 'â', 'ã', 'å', 'ā', 'ă', 'ą':
			r = 'a'
		case 'Ç', 'Ć', 'Ĉ', 'Ċ', 'Č':
			r = 'C'
		case 'ç', 'ć', 'ĉ', 'ċ', 'č':
			r = 'c'
		case 'É', 'È', 'Ê', 'Ë', 'Ē', 'Ĕ', 'Ė', 'Ę', 'Ě':
			r = 'E'
		case 'é', 'è', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
			r = 'e'
		case 'Í', 'Ì', 'Î', 'Ï', 'Ĩ', 'Ī', 'Ĭ', 'Į', 'İ':
			r = 'I'
		case 'í', 'ì', 'î', 'ï', 'ĩ', 'ī', 'ĭ', 'į', 'ı':
			r = 'i'
		case 'Ñ', 'Ń', 'Ņ', 'Ň':
			r = 'N'
		case 'ñ', 'ń', 'ņ', 'ň':
			r = 'n'
		case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö', 'Ø', 'Ō', 'Ŏ', 'Ő':
			r = 'O'
		case 'ó', 'ò', 'ô', 'õ', 'ö', 'ø', 'ō', 'ŏ', 'ő':
			r = 'o'
		case 'Ú', 'Ù', 'Û', 'Ü', 'Ũ', 'Ū', 'Ŭ', 'Ů', 'Ű', 'Ų':
			r = 'U'
		case 'ú', 'ù', 'û', 'ü', 'ũ', 'ū', 'ŭ', 'ů', 'ű', 'ų':
			r = 'u'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

func operatorByDisplayName(name string) (Operator, bool) {
	key := operatorNameKey(name)
	if key == "" {
		return 0, false
	}
	for op, knownName := range _Operator_map {
		if operatorNameKey(knownName) == key {
			return op, true
		}
	}
	return 0, false
}

func resolveHeaderPlayerOperator(p Player) Operator {
	if p.Operator != 0 {
		return p.Operator
	}
	if op, ok := operatorByDisplayName(p.RoleName); ok {
		return op
	}
	return p.Operator
}

func (h *Header) resolveHeaderOperators() {
	for i := range h.Players {
		h.Players[i].Operator = resolveHeaderPlayerOperator(h.Players[i])
	}
}

func (i Operator) fallbackNameForGameVersion(gameVersion string) string {
	switch operatorSeasonKey(gameVersion) {
	case "Y11S2":
		return "Y11S2UnknownOperator"
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
