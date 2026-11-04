package kitchen

import "github.com/simopzz/dandori/internal/web"

// The kitchen's Messages.
var (
	msgUpToDateAt = web.Message{En: "Kitchen up to date as of %s", It: "Cucina aggiornata alle %s"}
	msgUpToDateOn = web.Message{En: "Kitchen up to date as of %s, %s", It: "Cucina aggiornata %s alle %s"}
)
