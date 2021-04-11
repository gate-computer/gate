# Build-time extensions

Service implementations must be imported at build time.
The [cmd module](https://gate.computer/cmd) provides versions of the
gate-daemon and gate-server programs with a larger collection of services.

Go packages [implementing services](Service.md) may register themselves as Gate
extensions.  The convention is to register the extension at import (via the
`init` function or otherwise), and export the extension handle as `Ext`:

```go
package mything

import (
	"context"

	"gate.computer/gate/service"
)

type Config struct {
	// Arbitrary configuration keys (exported fields).
}

var extConfig = &Config{}

var Ext = service.Extend("mything", extConfig, func(ctx context.Context, r *service.Registry) error {
	// - Initialize services with configuration from extConfig.
	// - Register available services.
})
```

If sufficient configuration isn't provided for a service, the service should be
skipped instead of returning an error.

To build gate-daemon or gate-server with custom services, a Go main program
needs to be created:

```go
package main

import (
	_ "example.org/mything"
	m "gate.computer/gate/cmd/gate-server/main"
)

func main() { m.Main() }
```

If there are multiple extensions with the same name, they can be renamed to
resolve conflicts in configuration keys:

```go
package main

import (
	mythingnet "example.net/mything"
	mythingorg "example.org/mything"
	m "gate.computer/gate/cmd/gate-server/main"
)

func main() {
	mythingnet.Ext.Name = "mything-net"
	mythingorg.Ext.Name = "mything-org"
	m.Main()
}
```
