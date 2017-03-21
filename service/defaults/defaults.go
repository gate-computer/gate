package defaults

import (
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/peer"
)

func init() {
	echo.Register(nil)
	peer.Register(nil)
}
