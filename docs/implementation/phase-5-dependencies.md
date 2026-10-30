# Phase 5: dependency and license review

Reviewed on 2026-10-30 for the native local installation. This review covers source dependencies, not container images, operating-system packages, or a production runtime deployment.

## Vulnerability checks

The initial npm scan found five advisory entries affecting Ajv 8.17.1 and fast-uri 3.1.8. Updated Ajv to 8.18.0 and fast-uri to 3.1.9, regenerated the source-bundle catalog with its canonical generator, and retained the existing major versions. The final `pnpm audit --json` reports zero advisories in all severity categories.

The initial `govulncheck` scan found vulnerable standard-library calls on Go 1.26.0 and affected x/net code. The minimum Go patch is now 1.26.10. Updated x/net to 0.62.0 and its required x/sync, x/sys, and x/text versions. Final verification uses `govulncheck` v1.9.0. Scans reflect the advisory database at run time and are not a guarantee against undisclosed vulnerabilities.

The npm changes intentionally change the catalog implementation digest. Existing published versions keep their original digest. Before upgrading an installation with unfinished work, retain its matching binaries, catalog, and workers or finish that work first. Revalidate and publish a new draft revision for new admissions under this bundle; never rewrite an old publication to substitute the new digest. See the local upgrade procedure.

## License inventory and exceptions

[`evidence/phase-5-dependencies.json`](evidence/phase-5-dependencies.json) records compiled Go module versions and hashes of their license/notice files, plus installed npm license declarations. Regenerate with:

```sh
python3 scripts/dependency-inventory.py > docs/implementation/evidence/phase-5-dependencies.json
```

The Go source licenses inspected are predominantly Apache-2.0, MIT, BSD-style, and ISC; YAML includes MIT/Apache text. The installed npm set also includes MPL-2.0 (lightningcss and its native package), CC-BY-4.0 (caniuse-lite), Unlicense, and 0BSD. Preserve notices and review source/attribution obligations before redistributing binaries or modified third-party code.

Two source/metadata exceptions need explicit tracking:

- `unionfs` 4.6.0 has no `package.json` license field, but its installed `LICENSE` contains the Unlicense text. The inventory retains its file hash rather than silently treating missing metadata as approval.
- `github.com/nexus-rpc/nexus-proto-annotations` v0.1.0 contains no root license/notice file in the downloaded module. Its licensing evidence must be resolved before external redistribution. Do not infer permission from another Nexus repository.

The repository itself currently has no root LICENSE file. This work does not choose a project license on the owner's behalf. Local private verification can complete, but external distribution approval remains open.

`go-licenses` v2.0.0 and v2.0.1 failed to classify this Go toolchain's standard-library packages (missing module-info errors). The source-file inventory is the fallback evidence; it is not a claim that that classifier passed. The report covers imported Go modules, not every optional or unused module in the module graph, and npm native-package inventory is host-specific.
