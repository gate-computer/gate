package defaults

import (
	"github.com/tsavola/gate/service/echo"
)

func init() {
	echo.Register(nil)
}
