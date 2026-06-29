package cli

import (
	"fmt"
	"os"

	"github.com/4js-mikefolcher/fglpkg/internal/genero"
	"github.com/4js-mikefolcher/fglpkg/internal/installer"
	"github.com/4js-mikefolcher/fglpkg/internal/manifest"
	"github.com/4js-mikefolcher/fglpkg/internal/registry"
)

// cmdMigrate swaps one declared BDL dependency for another in the current
// project: it rewrites fglpkg.json, drops the old package, and reinstalls so
// the new one lands and the lock file is regenerated. This is a consumer-side
// helper — it changes *this* project's dependencies. It does not rename a
// published package or set up a registry-side redirect (that would require a
// registry endpoint and is out of scope).
//
//	fglpkg migrate <old> <new>[@version]   swap old → new (new defaults to latest)
//	fglpkg migrate <old> <new> --dry-run   show the change without writing
//
// The new package inherits the scope (prod/dev/optional) the old one lived in.
// Java dependencies are not yet supported.
func cmdMigrate(args []string) error {
	dryRun := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--dry-run", "-n":
			dryRun = true
		default:
			rest = append(rest, a)
		}
	}

	pkgArgs, forceLocal, forceGlobal, _ := parseFlags(rest)
	if len(pkgArgs) != 2 {
		return fmt.Errorf("usage: fglpkg migrate <old> <new>[@version] [--dry-run]")
	}
	old, newArg := pkgArgs[0], pkgArgs[1]

	home, _, err := resolveHome(forceLocal, forceGlobal)
	if err != nil {
		return err
	}

	m, err := manifest.Load(".")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no %s in current directory — run 'fglpkg init' first", manifest.Filename)
		}
		return fmt.Errorf("failed to load %s: %w", manifest.Filename, err)
	}

	// The old package must be a declared FGL dependency; we record which scope
	// it lives in so the replacement lands in the same bucket.
	oldConstraint, scope := m.FindFGLDependency(old)
	if scope == "" {
		return fmt.Errorf("%q is not a declared dependency in %s; nothing to migrate", old, manifest.Filename)
	}

	newName, newVersion, err := parsePackageArg(newArg)
	if err != nil {
		return err
	}
	if newName == old {
		return fmt.Errorf("old and new package are the same (%q); nothing to migrate", old)
	}

	gv, err := genero.Detect()
	if err != nil {
		return fmt.Errorf("cannot detect Genero version: %w", err)
	}
	generoMajor := gv.MajorString()

	// Resolve the replacement BEFORE mutating anything, so a bad/unknown new
	// package leaves the manifest and lock file untouched.
	fmt.Printf("Resolving %s@%s (Genero %s)...\n", newName, newVersion, gv)
	info, err := registry.Resolve(newName, newVersion, generoMajor)
	if err != nil {
		return fmt.Errorf("failed to resolve %s@%s: %w", newName, newVersion, err)
	}
	if info.Name == "" {
		info.Name = newName
	}

	scopeLabel := scopeDisplayName(scope)
	if dryRun {
		fmt.Println("\nDry run — no changes written:")
		fmt.Printf("  remove %s@%s from %s\n", old, oldConstraint, scopeLabel)
		fmt.Printf("  add    %s@%s to %s\n", info.Name, info.Version, scopeLabel)
		return nil
	}

	projectDir, _ := os.Getwd()
	inst := newInstaller(home)

	// Best-effort removal of the old artifact. It may be declared but not
	// currently installed (e.g. fresh checkout); that is not an error here —
	// the reinstall below re-resolves the manifest and rewrites the lock file.
	if err := inst.Remove(old); err != nil {
		fmt.Printf("Note: %s was not installed on disk; updating manifest only.\n", old)
	}

	m.RemoveFGLDependency(old)
	m.AddFGLDependencyScoped(info.Name, info.Version, scope)
	if err := m.Save("."); err != nil {
		return err
	}
	fmt.Printf("✓ Migrated %s → %s@%s in %s\n", old, info.Name, info.Version, scopeLabel)

	fmt.Println()
	if err := runHook(m, manifest.HookPreInstall, projectDir); err != nil {
		return err
	}
	if err := inst.InstallAllWithOptions(m, projectDir, true, installer.Options{}); err != nil {
		return err
	}
	return runHook(m, manifest.HookPostInstall, projectDir)
}
