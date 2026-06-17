package services

import "testing"

func TestMatchesTriggerReleasePublished(t *testing.T) {
	wf := &WorkflowDef{
		On: WorkflowTriggers{
			Release: &ReleaseTrigger{
				Types: []string{"published"},
			},
		},
	}

	if !MatchesTrigger(wf, "release", "v1.2.3", "published") {
		t.Fatal("expected published release event to match workflow trigger")
	}

	if MatchesTrigger(wf, "release", "v1.2.3", "created") {
		t.Fatal("expected created release event not to match published trigger")
	}
}

func TestMatchesTriggerReleaseTagFilter(t *testing.T) {
	wf := &WorkflowDef{
		On: WorkflowTriggers{
			Release: &ReleaseTrigger{
				Types: []string{"published"},
				Tags:  []string{"v*"},
			},
		},
	}

	if !MatchesTrigger(wf, "release", "v2.0.0", "published") {
		t.Fatal("expected matching tag to trigger release workflow")
	}

	if MatchesTrigger(wf, "release", "2.0.0", "published") {
		t.Fatal("expected non-matching tag not to trigger release workflow")
	}
}
