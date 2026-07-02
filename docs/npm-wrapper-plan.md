# npm wrapper (proposta, no implementada)

Objectiu: permetre `npx bonpreu-cli` / `npm install -g bonpreu-cli` sense requerir Go instal·lat.

## Com funcionaria

1. **`main.go`**: `const version` → `var version` (ja fet), perquè GoReleaser hi pugui
   injectar la versió amb `-ldflags -X main.version=...`.
2. **`.goreleaser.yaml`**: build multi-plataforma (linux/darwin/windows × amd64/arm64),
   publicant el binari cru (`format: binary`, sense tar/zip) per simplificar la descàrrega.
3. **`.github/workflows/release.yml`**: en push d'un tag `v*`, GoReleaser publica els
   binaris al GitHub Release corresponent.
4. **Paquet `npm/`**:
   - `package.json` amb `bin: { bonpreu: "bin/bonpreu.js" }`
   - `scripts/install.js`: hook `postinstall` que detecta OS/arch i descarrega el
     binari corresponent des del GitHub Release
   - `bin/bonpreu.js`: shim que executa el binari descarregat via `spawnSync`
5. Passos manuals per cada release: `git tag vX.Y.Z && git push --tags` (dispara el
   workflow) i `cd npm && npm publish` (cal sincronitzar `npm/package.json.version`
   amb el tag de git).

## Alternatives i pros/cons

| Opció | Pros | Cons |
|---|---|---|
| `go install` (actual) | Zero manteniment, ja funciona | Requereix Go instal·lat |
| npm wrapper (postinstall download) | `npx`/`npm i -g` sense Go | Cal publicar a npm + GitHub Releases; postinstall pot fallar sense xarxa; sincronitzar versió a mà |
| Homebrew tap | Natural per usuaris macOS/Linux CLI | No Windows; repo tap a part; audiència més reduïda |
| Script `curl \| sh` | Simple, sense registre extern | Menys "trustable"; detecció OS/arch a mà en bash |
| npm amb `optionalDependencies` per plataforma | Més robust (npm tria el paquet correcte sol) | N paquets a publicar i versionar cada release |

## Estat

No implementat. Si es decideix fer-ho, recuperar aquest pla i muntar `.goreleaser.yaml`,
`.github/workflows/release.yml` i el paquet `npm/`.
