package orientation_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/orientation"
)

// TestEveryListeningPortIsReadWhereTheCappedListStops: the shell tool's serve
// watches the socket tables for the port its server opens, and on a machine
// with forty listeners the capped list of twenty kept a high port out, so the
// serve said nothing listened while the server did.
func TestEveryListeningPortIsReadWhereTheCappedListStops(t *testing.T) {
	lines := []string{"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"}
	for at := range orientation.MaxPorts + 5 {
		lines = append(lines, fmt.Sprintf("   %d: 0100007F:%04X 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1 1 0000000000000000 100 0 0 10 0", at, 1000+at))
	}
	path := filepath.Join(t.TempDir(), "tcp")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("cannot write the table: %v", err)
	}

	capped, _ := orientation.ListeningPorts(path)
	every, problems := orientation.EveryListeningPort(path)
	if len(capped) != orientation.MaxPorts {
		t.Errorf("the capped list holds %d ports, want %d", len(capped), orientation.MaxPorts)
	}
	if len(every) != orientation.MaxPorts+5 || every[len(every)-1] != 1000+orientation.MaxPorts+4 {
		t.Errorf("the whole list holds %d ports ending at %d, want %d ending at %d", len(every), every[len(every)-1], orientation.MaxPorts+5, 1000+orientation.MaxPorts+4)
	}
	if len(problems) != 0 {
		t.Errorf("a readable table was reported as a problem: %v", problems)
	}
}
