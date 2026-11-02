package store

import "testing"

func TestRemoveSecrets(t *testing.T) {
	tests := []struct{ in, want string }{
		{
			`curl -H 'authorization: Bearer abc.def-123' https://x`,
			`curl -H 'authorization: Bearer [secret removed]' https://x`,
		},
		{"token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl here", "token [secret removed] here"},
		{"postgres://reef:hunter2@db:5432/x", "postgres://reef:[secret removed]@db:5432/x"},
		{"mongodb://admin:p4ss@mongo/x", "mongodb://admin:[secret removed]@mongo/x"},
		{"key sk-ant-FAKEFAKEFAKEFAKE1234", "key [secret removed]"},
		{"ghp_FAKEFAKEFAKEFAKEFAKE12 and glpat-FAKEFAKEFAKEFAKE12", "[secret removed] and [secret removed]"},
		{"slack xoxb-1234567890-FAKE", "slack [secret removed]"},
		{"run the task-runner, risk-free; postgres://db/x has no password", "run the task-runner, risk-free; postgres://db/x has no password"},
	}
	for _, tt := range tests {
		got, removed := RemoveSecrets(tt.in)
		if got != tt.want || removed != (tt.in != tt.want) {
			t.Errorf("RemoveSecrets(%q) = %q, %v; want %q", tt.in, got, removed, tt.want)
		}
		if again, removed := RemoveSecrets(got); again != got || removed {
			t.Errorf("RemoveSecrets(%q) again = %q, %v", got, again, removed)
		}
	}
}
