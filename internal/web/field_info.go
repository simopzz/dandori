package web

import (
	"strconv"
	"sync/atomic"

	"github.com/a-h/templ"
)

var fieldInfoSequence atomic.Uint64

// FieldInfoMark pairs `info` with its own popover target, so an outside tap can
// dismiss it.
func FieldInfoMark(info string) templ.Component {
	id := "field-info-" + strconv.FormatUint(fieldInfoSequence.Add(1), 10)
	return fieldInfoMark(id, info)
}
