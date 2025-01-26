package gate_test

import (
	"fmt"
	"sync"
	"time"

	"gate.computer/gate/principal"
	"gate.computer/gate/server/model"
	"google.golang.org/protobuf/proto"

	. "import.name/type/context"
)

type testInventory struct {
	mu        sync.Mutex
	modules   map[string][]byte
	instances map[string][]byte
}

func newTestInventory() *testInventory {
	return &testInventory{
		modules:   make(map[string][]byte),
		instances: make(map[string][]byte),
	}
}

func (db *testInventory) GetModule(ctx Context, pri principal.ID, key string, msg proto.Message) (found bool, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if buf, found := db.modules[fmt.Sprint(pri, key)]; found {
		return true, proto.Unmarshal(buf, msg)
	}
	return false, nil
}

func (db *testInventory) PutModule(ctx Context, pri principal.ID, key string, msg proto.Message) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	buf, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	db.modules[fmt.Sprint(pri, key)] = buf
	return nil
}

func (db *testInventory) UpdateModule(ctx Context, pri principal.ID, key string, msg proto.Message) error {
	return db.PutModule(ctx, pri, key, msg)
}

func (db *testInventory) RemoveModule(ctx Context, pri principal.ID, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.modules, fmt.Sprint(pri, key))
	return nil
}

func (db *testInventory) GetInstance(ctx Context, pri principal.ID, key string, msg proto.Message) (found bool, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if buf, found := db.instances[fmt.Sprint(pri, key)]; found {
		return true, proto.Unmarshal(buf, msg)
	}
	return false, nil
}

func (db *testInventory) PutInstance(ctx Context, pri principal.ID, key string, msg proto.Message) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	buf, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	db.instances[fmt.Sprint(pri, key)] = buf
	return nil
}

func (db *testInventory) UpdateInstance(ctx Context, pri principal.ID, key string, msg proto.Message) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.PutInstance(ctx, pri, key, msg)
}

func (db *testInventory) RemoveInstance(ctx Context, pri principal.ID, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.instances, fmt.Sprint(pri, key))
	return nil
}

type testSourceCache struct {
	mu      sync.Mutex
	sources map[string]string
}

func newTestSourceCache() *testSourceCache {
	return &testSourceCache{
		sources: make(map[string]string),
	}
}

func (db *testSourceCache) GetSourceSHA256(ctx Context, uri string) (hash string, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.sources[uri], nil
}

func (db *testSourceCache) PutSourceSHA256(ctx Context, uri, hash string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.sources[uri] = hash
	return nil
}

type testNonceChecker struct {
	mu     sync.Mutex
	nonces map[string]time.Time
}

func newTestNonceChecker() *testNonceChecker {
	return &testNonceChecker{
		nonces: make(map[string]time.Time),
	}
}

func (db *testNonceChecker) CheckNonce(ctx Context, scope []byte, nonce string, expires time.Time) error {
	now := time.Now()
	key := fmt.Sprint(scope, nonce)
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.nonces[key].After(now) {
		return model.ErrNonceReused
	}
	db.nonces[key] = now
	return nil
}
