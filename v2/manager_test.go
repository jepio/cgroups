package v2

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestEventChanCleanupOnCgroupRemoval(t *testing.T) {
	checkCgroupMode(t)

	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("failed to create cat process: ", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal("failed to start cat process:", err)
	}
	proc := cmd.Process
	if proc == nil {
		t.Fatal("process is nil")
	}

	group := fmt.Sprintf("testing-watcher-%d.scope", proc.Pid)
	c, err := NewSystemd("", group, proc.Pid, &Resources{})
	if err != nil {
		t.Fatal("failed to init new cgroup manager: ", err)
	}

	evCh, errCh := c.EventChan()

	data := `low 0
high 0
max 0
oom 0
oom_kill 0
`

	ticker := time.NewTicker(100 * time.Millisecond)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				err := os.WriteFile(filepath.Join(defaultCgroup2Path, defaultSlice, group, "memory.events"), []byte(data), 0o444)
				if !strings.HasSuffix(err.Error(), fs.ErrInvalid.Error()) {
					t.Logf("Unexpected error writing event to cgroup: %v", err)
					t.Fail()
				}
			}
		}
	}()

	for {
		finished := false
		select {
		case ev := <-evCh:
			t.Logf("event received: %v", ev)
			stdin.Close()
			cmd.Wait()
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Received error event: %v", err)
			}
			finished = true
		case <-time.After(time.Second * 15):
			t.Fatalf("No notification after 15s")
		}
		if finished {
			break
		}
	}
	t.Cleanup(func() {
		ticker.Stop()
		done <- true

		goleak.VerifyNone(t)
	})
}
