package spawner

import "testing"

func TestExpandSkillTurn(t *testing.T) {
	tests := []struct {
		name string
		m    *SkillMatch
		want string
	}{
		{
			name: "args empty returns body only",
			m:    &SkillMatch{Name: "finalize", Body: "FINALIZE BODY", Args: ""},
			want: "FINALIZE BODY",
		},
		{
			name: "args present appends delimited section",
			m:    &SkillMatch{Name: "do-release", Body: "RELEASE BODY", Args: "patch"},
			want: "RELEASE BODY\n\n---\nArguments: patch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandSkillTurn(tt.m); got != tt.want {
				t.Errorf("expandSkillTurn(%+v) = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}
