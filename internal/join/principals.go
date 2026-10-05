package join

import "github.com/SuperJackfruitLabs/superwitness/internal/auth"

// Principal ids come only from the producers' *_principal_id fields;
// superwitness never guesses one from a usr_, agt_ or other raw id.

// judgeKindOf mirrors verdicts.JudgeKindOf for display: service principals are graders.
func judgeKindOf(k auth.PrincipalKind) string {
	switch k {
	case auth.KindHuman:
		return "human"
	case auth.KindAgent:
		return "agent"
	case auth.KindService:
		return "grader"
	}
	return Unknown
}
