package command_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestDoctorPrintsTheReportAndSaysNothingIsBroken(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := &strings.Builder{}

	if !command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a fresh temporary home broken:\n%s", written)
	}
	for _, wanted := range []string{"coeus doctor", home.Root, "config.toml"} {
		if !strings.Contains(written.String(), wanted) {
			t.Errorf("the doctor's report leaves out %q:\n%s", wanted, written)
		}
	}
}

func TestDoctorSaysWhenTheConfigurationWillNotLoad(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.ConfigFile(), []byte("default_model = 17\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the broken configuration failed: %v", err)
	}
	written := &strings.Builder{}

	if command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a configuration that will not load fine:\n%s", written)
	}
	if !strings.Contains(written.String(), "default_model") {
		t.Errorf("the doctor's report does not name the key that is wrong:\n%s", written)
	}
}

func TestDoctorSaysWhenTheHomeFolderIsNotThere(t *testing.T) {
	home := emptyHome(t)
	written := &strings.Builder{}

	if command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a home folder that is not there fine:\n%s", written)
	}
	if !strings.Contains(written.String(), "coeus init") {
		t.Errorf("the doctor's report does not say to run coeus init:\n%s", written)
	}
}
