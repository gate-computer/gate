package run

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
)

var errExecutorDead = errors.New("executor died unexpectedly")

func uitoa(i uint) string {
	return strconv.FormatUint(uint64(i), 10)
}

// validateId makes sure that ids[i] is not root.
func validateId(name string, ids []uint, i int) (err error) {
	if ids[i] == 0 {
		err = fmt.Errorf("configured %s id #%d is 0", name, i)
	}
	return
}

// validateIds makes sure that ids don't conflict between themselves, with the
// current process, or root.
func validateIds(name string, ids []uint, untilIndex, currentId int) (err error) {
	for i, id := range ids[:untilIndex] {
		err = validateId(name, ids, i)
		if err != nil {
			return
		}

		if id == uint(currentId) {
			err = fmt.Errorf("configured %s id #%d is same as current %s id (%d)", name, i, name, currentId)
			return
		}
	}

	for i := range ids[:untilIndex] {
		for j := i + 1; j < len(ids); j++ { // all ids, not just untilIndex
			if ids[i] == ids[j] {
				err = fmt.Errorf("configured %s ids #%d and #%d are the same (%d)", name, i, j, ids[i])
				return
			}
		}
	}

	return
}

// checkCurrentGid makes sure that this process belongs to group configGids[i].
func checkCurrentGid(configGids []uint, i int) (err error) {
	currentGroups, err := syscall.Getgroups()
	if err != nil {
		return
	}

	currentGroups = append(currentGroups, syscall.Getgid())

	for _, currentGid := range currentGroups {
		if uint(currentGid) == configGids[i] {
			return
		}
	}

	err = fmt.Errorf("current process does not belong to configured group #%d (%d)", i, configGids[i])
	return
}

// recvEntry is like send_entry in executor.c
type recvEntry struct {
	Pid    int32 // pid_t
	Status int32
}

type execRequest struct {
	p     *process
	files []*os.File
}

type executor struct {
	conn            *net.UnixConn
	execRequests    chan execRequest
	killRequests    chan int32
	doneSending     chan struct{}
	doneReceiving   chan struct{}
	maxProcs        int64
	numProcs        int64 // atomic
	numProcsChanged chan struct{}
	lock            sync.Mutex
	pending         []*process
}

func (e *executor) init(config *Config) (err error) {
	err = validateIds("user", config.Uids[:], 2, syscall.Getuid())
	if err != nil {
		return
	}

	err = validateIds("group", config.Gids[:], 2, syscall.Getgid())
	if err != nil {
		return
	}

	err = validateId("group", config.Gids[:], 2)
	if err != nil {
		return
	}

	err = checkCurrentGid(config.Gids[:], 2)
	if err != nil {
		return
	}

	containerPath, err := filepath.Abs(config.container())
	if err != nil {
		return
	}

	executorFile, err := os.Open(config.executor())
	if err != nil {
		return
	}
	defer executorFile.Close()

	loaderFile, err := os.Open(config.loader())
	if err != nil {
		return
	}
	defer loaderFile.Close()

	fdPair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return
	}

	controlFile := os.NewFile(uintptr(fdPair[0]), "executor (peer)")
	defer controlFile.Close()

	connFile := os.NewFile(uintptr(fdPair[1]), "executor")
	defer connFile.Close()

	netConn, err := net.FileConn(connFile)
	if err != nil {
		return
	}

	cmd := &exec.Cmd{
		Path: containerPath,
		Args: []string{
			containerPath,
			uitoa(config.Uids[0]),
			uitoa(config.Gids[0]),
			uitoa(config.Uids[1]),
			uitoa(config.Gids[1]),
			uitoa(config.Gids[2]),
		},
		Env:    []string{},
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			controlFile,
			loaderFile,
			executorFile,
		},
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL,
		},
	}

	err = cmd.Start()
	if err != nil {
		return
	}

	e.conn = netConn.(*net.UnixConn)
	e.execRequests = make(chan execRequest) // no buffering; files must be closed
	e.killRequests = make(chan int32, 16)   // TODO: how much buffering?
	e.doneSending = make(chan struct{})
	e.doneReceiving = make(chan struct{})
	e.maxProcs = config.maxProcs()
	e.numProcsChanged = make(chan struct{}, 1)

	go e.sender()
	go e.receiver()
	go e.waiter(cmd)

	return
}

func (e *executor) execute(files []*os.File) (*process, error) {
	p := &process{
		e:      e,
		events: make(chan recvEntry, 2), // space for reply and status
	}

	select {
	case e.execRequests <- execRequest{p, files}:
		return p, nil

	case <-e.doneSending:

	case <-e.doneReceiving:
	}

	closeExecutionFiles(files)
	return nil, errExecutorDead
}

func (e *executor) close() error {
	select {
	case e.killRequests <- 0:
		<-e.doneSending

	case <-e.doneSending:
		// it died on its own
	}

	<-e.doneReceiving

	return e.conn.Close()
}

func (e *executor) sender() {
	var closed bool

	defer func() {
		if !closed {
			close(e.doneSending)
		}
	}()

	var numProcs int64

	for {
		var (
			execRequests <-chan execRequest
			buf          = make([]byte, 4) // sizeof (pid_t)
			files        []*os.File
			cmsg         []byte
		)

		if numProcs < e.maxProcs {
			execRequests = e.execRequests
		}

		select {
		case <-e.numProcsChanged:
			numProcs = atomic.LoadInt64(&e.numProcs)
			continue

		case exec := <-execRequests:
			e.lock.Lock()
			e.pending = append(e.pending, exec.p)
			e.lock.Unlock()

			numProcs++ // conservative estimate

			files = exec.files

			fds := make([]int, len(files))
			for i, f := range files {
				fds[i] = int(f.Fd())
			}
			cmsg = syscall.UnixRights(fds...)

		case pid := <-e.killRequests:
			if pid == 0 {
				close(e.doneSending)
				closed = true

				if err := e.conn.CloseWrite(); err != nil {
					log.Printf("executor socket: %v", err)
				}
				return
			}

			binary.LittleEndian.PutUint32(buf, uint32(pid)) // sizeof (pid_t)
		}

		_, _, err := e.conn.WriteMsgUnix(buf, cmsg, nil)
		if files != nil {
			closeExecutionFiles(files)
		}
		if err != nil {
			log.Printf("executor socket: %v", err)
			return
		}
	}
}

func (e *executor) receiver() {
	defer close(e.doneReceiving)

	r := bufio.NewReader(e.conn)
	running := make(map[int32]*process)

	var buf recvEntry

	for {
		if err := binary.Read(r, binary.LittleEndian, &buf); err != nil {
			if err != io.EOF {
				log.Printf("executor socket: %v", err)
			}
			return
		}

		var p *process

		if buf.Pid < 0 {
			e.lock.Lock()
			p = e.pending[0]
			e.pending = e.pending[1:]
			e.lock.Unlock()

			running[-buf.Pid] = p
		} else {
			p = running[buf.Pid]
			delete(running, buf.Pid)

			e.lock.Lock()
			pendingLen := len(e.pending)
			e.lock.Unlock()

			atomic.StoreInt64(&e.numProcs, int64(pendingLen+len(running)))

			select {
			case e.numProcsChanged <- struct{}{}:
			default:
			}
		}

		p.events <- buf
	}
}

func (e *executor) waiter(cmd *exec.Cmd) {
	err := cmd.Wait()

	if exit, ok := err.(*exec.ExitError); ok && exit.Success() {
		select {
		case <-e.doneSending:
			// clean exit
			return

		default:
			// unexpected exit
		}
	}

	log.Printf("executor process: %v", err)
}

type process struct {
	e      *executor
	events chan recvEntry
	pid    int32 // in another namespace
	killed bool
}

func (p *process) initPid() {
	if p.pid == 0 {
		entry := <-p.events
		p.pid = -entry.Pid
	}
}

func (p *process) kill() {
	if p.killed {
		return
	}

	p.initPid()

	select {
	case p.e.killRequests <- p.pid:
		p.killed = true

	case <-p.e.doneSending:

	case <-p.e.doneReceiving:
	}
}

func (p *process) killWait() (status syscall.WaitStatus, err error) {
	p.initPid()

	var killRequests chan<- int32
	if !p.killed {
		killRequests = p.e.killRequests
	}

	for {
		select {
		case killRequests <- p.pid:
			killRequests = nil
			p.killed = true

		case <-p.e.doneSending:
			// no way to kill it anymore
			killRequests = nil

		case entry := <-p.events:
			status = syscall.WaitStatus(entry.Status)
			return

		case <-p.e.doneReceiving:
			err = errExecutorDead
			return
		}
	}
}
