# IA METER Collector — Implementation Plan

Status legend: `[ ]` pending · `[~]` in progress · `[x]` done

## Phase 0 — Inspection & Planning — `[x]`

### Diagnóstico del repositorio
- Directorio vacío, sin `.git`. Repo inicializado en esta fase (`git init`).
- No existían `README.md`, `AGENTS.md`, `CLAUDE.md` ni convenciones previas — no hay contratos existentes que preservar.
- Go 1.22.2 disponible en el entorno (`go version go1.22.2 linux/amd64`).
- No hay pruebas previas que ejecutar (proyecto greenfield).

### Verificación contra documentación oficial
Se consultó `https://code.claude.com/docs/en/statusline` (redirect desde `docs.anthropic.com`). Confirmado:
- El statusLine recibe JSON por stdin con, entre otros, un objeto `rate_limits` opcional.
- `rate_limits.five_hour.used_percentage`, `rate_limits.seven_day.used_percentage`: 0–100.
- `rate_limits.five_hour.resets_at`, `rate_limits.seven_day.resets_at`: unix epoch seconds.
- `rate_limits` **solo aparece para suscriptores Claude.ai Pro/Max, después de la primera respuesta de API en la sesión**. Cada ventana (`five_hour`, `seven_day`) puede estar ausente de forma independiente.
- Configuración en `settings.json`: `{"statusLine": {"type": "command", "command": "<cmd>"}}`. Confirma la forma esperada en la sección 13 del prompt original.
- No se encontraron contradicciones entre el prompt y la documentación oficial actual. No fue necesario registrar excepciones.

### Arquitectura propuesta
Módulo Go único (`github.com/iameter/collector`), binario único `iameter`. Paquetes internos separados por responsabilidad (ver sección "Estructura" abajo), sin abstracciones sin uso. Proveedor de uso desacoplado vía interfaz `UsageProvider` para permitir proveedores futuros sin tocar cola/sync/pairing/daemon/installers.

### Riesgos técnicos identificados
1. **No hay backend real.** Se implementa un mock server HTTP (`internal/mockserver` + `cmd/iameter-mockserver`, o subcomando dev) para poder probar pairing/sync end-to-end sin depender de infraestructura externa. Documentado como modo desarrollo, nunca presentado como producción.
2. **Almacenes de credenciales nativos (DPAPI, Keychain, Secret Service) no son verificables en este entorno Linux headless de CI/sandbox.** Se implementan con build tags por SO; en Linux se usa D-Bus Secret Service vía llamadas a `secret-tool`/dbus si está disponible, con fallback a archivo con permisos `0600`. Las implementaciones de Windows/macOS se compilan solo en cross-compile (no se pueden ejecutar pruebas de integración reales en este entorno) — se documenta como limitación real en Fase 8.
3. **systemd --user / LaunchAgent / Scheduled Task no se pueden registrar ni verificar en este sandbox** (sin systemd de usuario activo, sin macOS/Windows). Se implementa generación de unit files / plist / schtasks command y pruebas unitarias del contenido generado, pero no un smoke test real del SO. Documentado como limitación.
4. **Cross-compilation de 6 targets**: sin CGO se puede cross-compilar todo desde Linux con `GOOS`/`GOARCH`. Se evita CGO en credenciales (usar bindings a herramientas de línea de comando / syscalls puros de `golang.org/x/sys` en vez de cgo-keychain) para mantener esto simple.
5. **Firma de código / notarización fuera de alcance** (sección 34) — documentado en README/SECURITY como requisito para distribución pública real.
6. **Dato ausente vs. cero**: se usa `*float64`/punteros y struct con flags de presencia en vez de valores centinela, para no confundir 0% real con ausencia.

### Decisiones iniciales
- Go estándar, sin frameworks CLI externos pesados; se implementa un router de subcomandos manual en `internal/cli` (evita dependencia de cobra para "dependencias externas mínimas"). Única dependencia externa que se evalúa: ninguna obligatoria para MVP; toda la lógica con stdlib (`encoding/json`, `net/http`, `flag`, `os`, `path/filepath`, `crypto/sha256`, etc).
- Versionado inyectado por `-ldflags` (`version.Version`, `version.Commit`, `version.BuildDate`), default `0.1.0-dev`.
- `device_id` generado localmente (UUID v4 vía `crypto/rand`, sin dependencias) y persistido en `config-dir`.
- Cola en disco como archivo JSONL append-only + compactación, con `flock`/lockfile propio multiplataforma (archivo `.lock` + `O_EXCL`, ya que `syscall.Flock` no es portable a Windows sin cgo — se implementa lock propio basado en creación exclusiva de archivo con PID + staleness check).

### Estado de pruebas existentes
N/A — proyecto nuevo, sin pruebas previas.

---

## Phase 1 — Núcleo, CLI y modelos — `[ ]`
Criterios de aceptación: compila; `iameter version/status/doctor/statusline/pair/sync/daemon/install/uninstall/unpair` existen y responden (parcial permitido, pero explícito); flags globales parseados; logging no filtra secretos.

## Phase 2 — Captura statusLine — `[ ]`
Criterios: parser cumple lista blanca estricta; 17 casos de prueba de sección 26 "Parser" cubiertos; fixtures de sección 27 creados; sin red obligatoria; ausencia ≠ cero.

## Phase 3 — Config Claude Code / instalación local — `[ ]`
Criterios: instala/preserva/encadena statusLine previo; backup; idempotente; restaura al desinstalar; soporta rutas con espacios/Unicode; nunca sobrescribe JSON corrupto.

## Phase 4 — Cola local / offline — `[ ]`
Criterios: escritura atómica; dedup; recuperación ante corrupción; límite de tamaño; compactación; concurrente-seguro.

## Phase 5 — Emparejamiento y backend — `[ ]`
Criterios: pairing contra mock server; credenciales nunca en texto plano en logs; cliente HTTP con reintentos/backoff/timeouts/idempotencia; pruebas httptest de todos los códigos de sección 26.

## Phase 6 — Daemon — `[ ]`
Criterios: single-instance; graceful shutdown; heartbeat; backoff+jitter; respeta Retry-After; detiene reintentos en 401/403; `iameter sync` sincroniza una vez y termina.

## Phase 7 — Instaladores y distribución — `[ ]`
Criterios: 4 scripts creados; detectan SO/arch; verifican SHA-256; rollback; CI compila 6 targets + checksums.

## Phase 8 — Endurecimiento, documentación, validación final — `[ ]`
Criterios: SECURITY.md, PRIVACY.md completos y verídicos; `go build ./...`, `go vet ./...`, `go test ./...` pasan; 6 binarios compilados; privacidad verificada contra código.

---

## Limitaciones reales conocidas (actualizado en Fase 8)
Ver sección final de este documento, completada al cierre de Fase 8.
