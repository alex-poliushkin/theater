package theatercli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDebugBreakpointFileIgnoresCommentLinesAndPreservesHashesInsideSelectors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "team.debug")
	if err := os.WriteFile(path, []byte(
		"# comment only\n"+
			"   kind=action,phase=before,path=stage.main/call.login#1/act.submit/action\n"+
			"# retry-aware selector; attempt filters are valid\n"+
			"kind=expectation,phase=after,path=**/expectation.token\n"+
			"\n",
	), 0o600); err != nil {
		t.Fatalf("write break file failed: %v", err)
	}

	got, err := loadDebugBreakpointFile(path)
	if err != nil {
		t.Fatalf("loadDebugBreakpointFile error = %v", err)
	}

	want := []string{
		"kind=action,phase=before,path=stage.main/call.login#1/act.submit/action",
		"kind=expectation,phase=after,path=**/expectation.token",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadDebugBreakpointFile mismatch:\n got %#v\nwant %#v", got, want)
	}
}
