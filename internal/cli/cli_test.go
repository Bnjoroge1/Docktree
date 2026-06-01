package cli

import "testing"

func TestParseComposeRunState(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    composeRunState
		wantErr bool
	}{
		{
			name: "running entry",
			out:  `[{"State":"running"}]`,
			want: composeRunRunning,
		},
		{
			name: "stopped entry",
			out:  `[{"State":"exited"}]`,
			want: composeRunStopped,
		},
		{
			name:    "invalid json",
			out:     `not json`,
			want:    composeRunUnknown,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseComposeRunState(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if got != tt.want {
					t.Fatalf("state = %v, want %v", got, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("state = %v, want %v", got, tt.want)
			}
		})
	}
}
