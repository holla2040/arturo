// Command arturo-supervisor keeps the three Arturo services running and
// restarts them automatically whenever their executables change on disk.
//
// Usage:
//
//	arturo-supervisor [-dir ../services]
//
// The supervisor watches the controller, console, and terminal binaries in
// the given directory using inotify.  When a binary is written (e.g. after
// "go build"), the supervisor kills the running process and relaunches it.
// Ctrl-C or SIGTERM shuts everything down cleanly.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// service describes one managed binary.
type service struct {
	name    string   // human label
	bin     string   // absolute path to executable
	args    []string // extra CLI flags
	workDir string   // working directory (empty = inherit)

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{} // closed when process exits
}

func main() {
	dir := flag.String("dir", "", "directory containing the service binaries (default: ../services relative to this binary)")
	flag.Parse()

	if *dir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("cannot determine executable path: %v", err)
		}
		*dir = filepath.Join(filepath.Dir(exe), "..", "..", "services")
	}
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("bad directory: %v", err)
	}

	services := []*service{
		{name: "controller", bin: filepath.Join(absDir, "controller")},
		{name: "console", bin: filepath.Join(absDir, "console")},
		{name: "terminal", bin: filepath.Join(absDir, "terminal"), args: []string{"-dev"}, workDir: absDir},
	}

	// Verify all binaries exist before starting.
	for _, s := range services {
		if _, err := os.Stat(s.bin); err != nil {
			log.Fatalf("%s: %v", s.name, err)
		}
	}

	// Set up inotify watcher on the directory.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("fsnotify: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Add(absDir); err != nil {
		log.Fatalf("watch %s: %v", absDir, err)
	}

	// Map basename -> service for quick lookup on file events.
	byBase := make(map[string]*service)
	for _, s := range services {
		byBase[filepath.Base(s.bin)] = s
	}

	// Start all services.
	for _, s := range services {
		startService(s)
	}

	// Handle OS signals for clean shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// debounce timers per service — builds often cause multiple writes.
	debounce := make(map[string]*time.Timer)

	log.Println("supervisor ready — watching", absDir)

	for {
		select {
		case sig := <-sigCh:
			log.Printf("received %v, shutting down all services", sig)
			for _, s := range services {
				stopService(s)
			}
			return

		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			// We care about writes and creates (rename-into counts as create).
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			base := filepath.Base(ev.Name)
			s, ok := byBase[base]
			if !ok {
				continue
			}
			// Debounce: wait 500ms after the last write before restarting,
			// because go build can produce multiple write events.
			if t, exists := debounce[base]; exists {
				t.Stop()
			}
			debounce[base] = time.AfterFunc(500*time.Millisecond, func() {
				log.Printf("%s binary changed — restarting", s.name)
				stopService(s)
				startService(s)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

// startService launches the binary as a child process.
func startService(s *service) {
	s.mu.Lock()
	defer s.mu.Unlock()

	args := s.args
	cmd := exec.Command(s.bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if s.workDir != "" {
		cmd.Dir = s.workDir
	}
	// Give each child its own process group so we can kill it cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Printf("start %s: %v", s.name, err)
		return
	}

	s.cmd = cmd
	s.done = make(chan struct{})
	log.Printf("started %s (pid %d)", s.name, cmd.Process.Pid)

	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.cmd = nil
		s.mu.Unlock()
		close(s.done)
		if err != nil {
			log.Printf("%s exited: %v", s.name, err)
		} else {
			log.Printf("%s exited cleanly", s.name)
		}
	}()
}

// stopService sends SIGTERM then SIGKILL after a grace period.
func stopService(s *service) {
	s.mu.Lock()
	cmd := s.cmd
	done := s.done
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	pid := cmd.Process.Pid
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// Process already gone.
		return
	}

	log.Printf("stopping %s (pid %d)", s.name, pid)
	// Kill the entire process group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// Wait up to 5 seconds for graceful exit.
	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		log.Printf("%s did not exit in 5s, sending SIGKILL", s.name)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
	}
}

func init() {
	log.SetFlags(log.Ltime)
	log.SetPrefix(fmt.Sprintf("[supervisor] "))
}
