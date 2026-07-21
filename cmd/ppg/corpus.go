package main

import (
	"fmt"
	"log"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/catalog"
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/skill"
)

// corpusConfig captures the CLI flags that determine what loadCorpus reads
// from disk, so startup and SIGHUP hot reload share one loading path.
type corpusConfig struct {
	adrDir           string
	skillsDir        string
	skillGovDir      string
	servicesDir      string
	servicePolicyDir string
	allowWideScope   bool
}

// corpus bundles every policy-derived dependency of the HTTP handlers.
type corpus struct {
	store     *adr.Store
	lint      *linter.Linter
	skillLint *skill.Linter
	catStore  *catalog.Store
	ranker    *catalog.Ranker
}

// loadCorpus reads the whole governance corpus from disk. It returns errors
// instead of exiting so the two callers can differ: at startup an error is
// fatal; on SIGHUP reload it is logged and the previous corpus keeps
// serving (fail-safe — a half-written policy on disk must not take the
// validation server down).
func loadCorpus(cfg corpusConfig) (*corpus, error) {
	// The ADR corpus is optional: organization-wide invariants are one
	// source of policies; skill companions (-skills, POST /register_skill)
	// are another. Without -adr the validation server runs on skill
	// policies and built-in rules alone (the tutorial-15 shape).
	var store *adr.Store
	if cfg.adrDir == "" {
		store = &adr.Store{}
		log.Printf("ADR store: none (-adr omitted) — skill companions and built-in rules only")
	} else {
		var err error
		store, err = adr.Load(cfg.adrDir)
		if err != nil {
			return nil, fmt.Errorf("loading ADR store: %w", err)
		}
		// filepath.Glob succeeds silently on a missing directory, so an empty
		// store means a typo'd -adr path, not a valid corpus.
		if len(store.Invariants) == 0 {
			return nil, fmt.Errorf("no ADRs (*.md) found in %s — check the -adr path", cfg.adrDir)
		}
		log.Printf("ADR store loaded: %d invariants", len(store.Invariants))
	}

	lint, err := linter.New(store, cfg.adrDir)
	if err != nil {
		return nil, fmt.Errorf("loading plan linter: %w", err)
	}
	lint.AllowWideScope = cfg.allowWideScope
	if cfg.allowWideScope {
		log.Printf("WARNING: -allow-wide-scope set; root-scoped plans yield allow-all tickets")
	}
	log.Printf("Plan linter ready: %d policies", len(lint.Registry))

	// Gate 3: plans that declare skill_id are additionally evaluated against
	// the published skill's companion Rego. Without -skills, any skill_id is
	// an unknown_skill rejection (fail closed) unless the session uploaded
	// the skill via POST /register_skill.
	if cfg.skillsDir != "" {
		if err := lint.LoadSkillCompanions(cfg.skillsDir); err != nil {
			return nil, fmt.Errorf("loading skill companions: %w", err)
		}
		log.Printf("Skill companions loaded (Gate 3): %d skills", lint.SkillCount())
	}

	skillLint, err := skill.NewLinter(cfg.skillGovDir)
	if err != nil {
		return nil, fmt.Errorf("loading skill governance linter: %w", err)
	}
	log.Printf("Skill governance linter ready")

	// The service catalog is an optional capability: without -services the
	// gateway serves everything except discovery.
	var catStore *catalog.Store
	var ranker *catalog.Ranker
	switch {
	case cfg.servicesDir == "" && cfg.servicePolicyDir == "":
		log.Printf("Service catalog disabled (no -services); /discover_service will answer SERVICE_CATALOG_UNAVAILABLE")
	case cfg.servicesDir == "":
		return nil, fmt.Errorf("-service-policy requires -services")
	default:
		catStore, err = catalog.Load(cfg.servicesDir)
		if err != nil {
			return nil, fmt.Errorf("loading service catalog: %w", err)
		}
		if len(catStore.All()) == 0 {
			return nil, fmt.Errorf("no service records (*.md) found in %s — check the -services path", cfg.servicesDir)
		}
		log.Printf("Service catalog loaded: %d services", len(catStore.All()))
		if cfg.servicePolicyDir == "" {
			log.Printf("WARNING: no -service-policy given; catalog loaded but /discover_service is disabled")
		} else {
			ranker, err = catalog.NewRanker(cfg.servicePolicyDir)
			if err != nil {
				return nil, fmt.Errorf("loading service ranking policy: %w", err)
			}
		}
	}

	return &corpus{store: store, lint: lint, skillLint: skillLint, catStore: catStore, ranker: ranker}, nil
}
