package account

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// This opt-in control proves the password-comparison assertion fails when the
// actual comparison is bypassed, and restores the source before returning.
func TestPasswordComparisonMutationControl(t *testing.T) {
	if os.Getenv("ACCOUNT_COMPARISON_CONTROL") != "1" {
		t.Skip("set ACCOUNT_COMPARISON_CONTROL=1 to run disposable comparison-bypass control")
	}
	path := "repository_account.go"
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(original), "_ = bcryptCompare(password, dummyPasswordHash)", "// bypassed bcrypt comparison", 1)
	if mutated == string(original) {
		t.Fatal("comparison bypass control could not locate unknown-account bcrypt comparison")
	}
	if err := os.WriteFile(path, []byte(mutated), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Errorf("restore password comparison: %v", err)
		}
	}()
	command := exec.Command("go", "test", "-run", "^TestValidateCredentialsComparesUnknownAndWrongPassword$", "-count=1", ".")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("bypassing bcrypt comparison unexpectedly passed: %s", output)
	}
	if !strings.Contains(string(output), "unknown account must perform a bcrypt comparison") {
		t.Fatalf("comparison bypass failed for an unintended reason: %s", output)
	}
	t.Logf("comparison bypass rejected by deterministic comparison-count assertion: %s", strings.TrimSpace(string(output)))
}
