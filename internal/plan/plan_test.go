package plan

import "testing"

func validBase() *Plan {
	return &Plan{
		SessionID: "11111111-1111-1111-1111-111111111111",
		Intent:    "Add the Seka payment method",
		RepositoryContext: RepoContext{
			Name:      "checkout-service",
			TechStack: []string{"Go"},
		},
		Steps: []Step{
			{ID: "s1", Action: "edit", Tool: "patch_code", Targets: []string{"a.go"}},
		},
	}
}

func TestValidateStructureAcceptsValidPlan(t *testing.T) {
	p := validBase()
	p.Steps = append(p.Steps, Step{
		ID: "s2", Action: "test", Tool: "go-test", Targets: []string{"a_test.go"},
		DependsOn: []string{"s1"},
	})
	if err := p.ValidateStructure(); err != nil {
		t.Fatalf("expected valid plan, got %v", err)
	}
}

func TestValidateStructureRejectsDuplicateID(t *testing.T) {
	p := validBase()
	p.Steps = append(p.Steps, Step{ID: "s1", Action: "again", Tool: "patch_code", Targets: []string{"b.go"}})
	if err := p.ValidateStructure(); err == nil {
		t.Fatal("expected duplicate step id to be rejected")
	}
}

func TestValidateStructureRejectsDanglingDependsOn(t *testing.T) {
	p := validBase()
	p.Steps[0].DependsOn = []string{"ghost"}
	if err := p.ValidateStructure(); err == nil {
		t.Fatal("expected dangling depends_on to be rejected")
	}
}

func TestValidateStructureRejectsSelfDependency(t *testing.T) {
	p := validBase()
	p.Steps[0].DependsOn = []string{"s1"}
	if err := p.ValidateStructure(); err == nil {
		t.Fatal("expected self-dependency to be rejected")
	}
}

func TestValidateStructureRejectsCycle(t *testing.T) {
	p := validBase()
	p.Steps = []Step{
		{ID: "s1", Action: "a", Tool: "patch_code", Targets: []string{"a.go"}, DependsOn: []string{"s2"}},
		{ID: "s2", Action: "b", Tool: "patch_code", Targets: []string{"b.go"}, DependsOn: []string{"s1"}},
	}
	if err := p.ValidateStructure(); err == nil {
		t.Fatal("expected dependency cycle to be rejected")
	}
}
