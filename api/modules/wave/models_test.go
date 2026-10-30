package wave

import "testing"

func TestValidTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{Todo, InProgress, true}, {InProgress, Review, true}, {Review, Done, true},
		{Done, Todo, false}, {Todo, Done, false}, {Blocked, Review, false},
		{Review, InProgress, true}, {Todo, Todo, true},
	}
	for _, tt := range tests {
		if got := validTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("validTransition(%q,%q)=%v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestValidState(t *testing.T) {
	for _, state := range []string{Todo, InProgress, Review, Blocked, Done} {
		if !validState(state) {
			t.Errorf("state %q rejected", state)
		}
	}
	if validState("running") {
		t.Fatal("unknown state accepted")
	}
}
