package auth

import "github.com/simopzz/dandori/internal/web"

// Signing in's Messages.
var (
	msgSignIn       = web.Message{En: "Sign in", It: "Accedi"}
	msgSignInTitle  = web.Message{En: "Sign in · Dandori", It: "Accedi · Dandori"}
	msgByInvitation = web.Message{
		En: "Dandori is by invitation. Sign in with the email you were invited with.",
		It: "Dandori è su invito. Accedi con l'email a cui è arrivato l'invito.",
	}
	msgBadLink = web.Message{
		En: "That sign-in link has expired or was already used. Ask for a new one.",
		It: "Quel link di accesso è scaduto o è già stato usato. Chiedine uno nuovo.",
	}
	msgNotSent = web.Message{
		En: "The sign-in link couldn't be sent. Try again in a moment.",
		It: "Non è stato possibile inviare il link di accesso. Riprova tra poco.",
	}
	msgEmail        = web.Message{En: "Email", It: "Email"}
	msgNotAnEmail   = web.Message{En: "Enter the email you were invited with.", It: "Inserisci l'email a cui è arrivato l'invito."}
	msgSendLink     = web.Message{En: "Email me a sign-in link", It: "Inviami un link di accesso"}
	msgCheckEmail   = web.Message{En: "Check your email", It: "Controlla la tua email"}
	msgCheckTitle   = web.Message{En: "Check your email · Dandori", It: "Controlla la tua email · Dandori"}
	msgLinkOnItsWay = web.Message{
		En: "If that email was invited, a sign-in link is on its way.",
		It: "Se quell'email è stata invitata, un link di accesso è in arrivo.",
	}
	msgOpenItHere = web.Message{
		En: "Open it in the browser you want to use Dandori in: that is where it signs you in. If none arrives in a few minutes, ask again.",
		It: "Aprilo nel browser in cui vuoi usare Dandori: è lì che ti fa accedere. Se non arriva entro qualche minuto, chiedine un altro.",
	}
	msgAnotherEmail = web.Message{En: "Use another email", It: "Usa un'altra email"}
	msgSignInTo     = web.Message{En: "Sign in to Dandori", It: "Accedi a Dandori"}
)
