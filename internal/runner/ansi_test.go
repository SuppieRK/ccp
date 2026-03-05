package runner

import "testing"

func TestStripANSI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "csi color",
			in:   "\x1b[31merror\x1b[0m\n",
			want: "error\n",
		},
		{
			name: "multiple csi",
			in:   "\x1b[1m\x1b[32msuccess\x1b[0m\n",
			want: "success\n",
		},
		{
			name: "plain text",
			in:   "no ansi\n",
			want: "no ansi\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripANSI(tc.in); got != tc.want {
				t.Fatalf("stripANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
