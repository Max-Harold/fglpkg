# fglpkg — Executive Summary

**A package manager and module registry for Genero BDL — for our Professional Services teams, our end-user customers, and the AI agents that will increasingly build with them.**

## What is a package manager?

Every modern language ecosystem has a package manager — and Genero BDL has been missing one. A package manager is the tool that lets a developer install a library by name and version, get a reproducible build, audit dependencies for known security issues, and share what they wrote with the next team that needs it. **npm** did this for JavaScript, **Maven** for Java, **pip** for Python — each reached near-universal use within a few years of arrival because the productivity gain is large and obvious. Genero has lived without one for its entire history.

## The problem

Genero BDL developers in 2026 still copy `.42m` files between projects, ship Java JARs by hand, track versions in release notes, and cannot answer "do any of my dependencies have a known CVE?" Every PS engagement re-solves these problems from scratch. Every end-user customer's BDL stack rots quietly between releases. Every module written for one team never reaches another who could reuse it. The cost shows up as **lost PS hours, missed reuse across the customer base, and a security-review blocker** the moment a Genero project hits an enterprise procurement team.

## The insight

Genero has the same dependency-management problem npm and Maven solved 15+ years ago — but it has one differentiator nobody else has: **the Genero major-version variant**. We can ship a tool that's *simpler* than npm (BDL has no JS/TS-singleton problem) and *more honest* than Maven (Genero versioning is first-class, not a coordinate suffix). The architecture is validated end-to-end: a Fly.io metadata registry, packages stored as GitHub Release assets, a Go CLI that resolves BDL and Java in one dependency graph, monorepo workspaces, lockfile, and OSV.dev vulnerability audit.

**And one thing none of the older package managers were designed for: AI-native discovery.** fglpkg ships with an MCP integration so AI coding agents can search the registry, audit dependencies, and install packages on a developer's behalf — turning *"I need to parse Excel in my Genero app"* into a fully wired-up project without a human gluing the pieces together. Every Genero developer — junior, senior, or AI agent — gets the same leverage.

## The value

- **Professional Services productivity** — shared utilities reused in one command instead of rebuilt every engagement. Hours redeployed from re-work to billable customer outcomes.
- **End-user customer teams** — the same registry, the same audit, the same reproducibility. A Genero shop maintaining its own BDL stack gets dependency hygiene it could never assemble alone.
- **Enterprise procurement unlock** — live CVE audit against OSV.dev clears the security-review gate that today blocks Genero from regulated customers.
- **Agentic AI leverage** — MCP-aware from day one. AI agents discover, audit, and install packages autonomously, accelerating every developer regardless of seniority and putting Genero on the modern AI-native baseline.
- **Platform & ecosystem** — the registry that powers the CLI today is one step from a public web registry that turns Genero from a closed ecosystem into a community.
- **Strategic de-risking** — PS validates demand internally, generates concrete ROI data, and builds the case for broader R&D investment without committing the product roadmap up front.

> **Status:** v2.0.1 shipping. All P0 market-readiness items in or landing this cycle. MCP integration in flight. Already available to PS today; rolls out to end-user customers next.
