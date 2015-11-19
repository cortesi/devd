package devd

import (
	"fmt"
	"net/http"

	"github.com/cortesi/termlog"
	"github.com/fatih/color"
)

// LogHeader logs a header
func LogHeader(log termlog.Logger, h http.Header) {
	max := 0
	for k := range h {
		if len(k) > max {
			max = len(k)
		}
	}
	for k, vals := range h {
		for _, v := range vals {
			pad := fmt.Sprintf(fmt.Sprintf("%%%ds", max-len(k)+1), " ")
			log.SayAs(
				"headers",
				"\t%s%s%s",
				color.BlueString(k)+":",
				pad,
				v,
			)
		}
	}
}
