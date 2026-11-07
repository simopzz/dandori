package web

import "github.com/simopzz/dandori/internal/domain"

// The shell's and the shared components' Messages.
var (
	msgKitchen        = Message{En: "Kitchen", It: "Cucina"}
	msgProfile        = Message{En: "Profile", It: "Profilo"}
	msgMainNav        = Message{En: "Main", It: "Principale"}
	msgLanguageSwitch = Message{En: "Language", It: "Lingua"}
	msgChange         = Message{En: "Change", It: "Modifica"}
	msgClose          = Message{En: "Close", It: "Chiudi"}
	msgCloseNotice    = Message{En: "Close notice", It: "Chiudi avviso"}
)

// problems say why a typed number is refused, the same for every field.
var problems = map[domain.Problem]Message{
	domain.NotANumber: {En: "Type a number.", It: "Scrivi un numero."},
	domain.Negative:   {En: "Can't be below 0.", It: "Non può essere sotto 0."},
	domain.TooPrecise: {En: "One decimal place at most.", It: "Al massimo un decimale."},
}

// ProblemMessage says why a typed value is refused for problem: as own says, a field's
// own wording, or else as every field does.
func ProblemMessage(problem domain.Problem, own map[domain.Problem]Message) Message {
	if m, ok := own[problem]; ok {
		return m
	}
	return problems[problem]
}
