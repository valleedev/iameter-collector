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

## Phase 1 — Núcleo, CLI y modelos — `[x]`
Criterios de aceptación: compila; `iameter version/status/doctor/statusline/pair/sync/daemon/install/uninstall/unpair` existen y responden (parcial permitido, pero explícito); flags globales parseados; logging no filtra secretos.

**Archivos creados:** `go.mod`, `cmd/iameter/main.go`, `internal/version/{version,runtime}.go`, `internal/model/model.go`, `internal/platform/{platform,dirs}.go`, `internal/logging/logging.go`, `internal/fsutil/{atomic,lock,process_unix,process_windows}.go`, `internal/device/device.go`, `internal/config/{options,device}.go`, `internal/cli/{cli,globals,version_cmd,status_cmd,doctor_cmd,stubs}.go`.

**Funcionalidad implementada:**
- Router de subcomandos manual (sin cobra), 10 comandos registrados. `version`, `status`, `doctor` con lógica real; `statusline/pair/sync/daemon/install/uninstall/unpair` responden con mensaje explícito "not yet implemented (lands in Phase N)" y exit code 1 — nunca fingen éxito.
- Flags globales (`--api-base-url`, `--config-dir`, `--data-dir`, `--log-level`, `--json`, `--no-color`) funcionan antes **o** después del subcomando (`splitArgs` los reordena).
- `IAMETER_API_BASE_URL` respetado; default apunta a `http://127.0.0.1:8787` (mock dev, nunca presentado como producción — `doctor`/`status` emiten WARN explícito si sigue en default).
- Directorios por SO (XDG en Linux, `Library/Application Support` en macOS, `%LOCALAPPDATA%` en Windows) vía `internal/platform`.
- `device_id` generado localmente (`dev_` + 8 bytes aleatorios base32), persistido en `device.json` con escritura atómica (`internal/fsutil.AtomicWriteFile`: temp file + fsync + rename) y permisos `0600`.
- Lock de archivo entre procesos (`internal/fsutil.FileLock`) con detección de locks obsoletos (proceso muerto vía signal-0 en Unix, expiración por tiempo en todas las plataformas) — reutilizado por la cola en Fase 4.
- Logger propio con `RedactToken()` (nunca imprime tokens completos).

**Pruebas ejecutadas:** `go test ./...` (18 tests, todos en package), `go vet ./...`, `gofmt -l .` — todo limpio. Cross-compilación verificada para los 6 targets (`linux/{amd64,arm64}`, `windows/{amd64,arm64}`, `darwin/{amd64,arm64}`) con `CGO_ENABLED=0`, todos compilan sin error.

**Decisiones técnicas:**
- Sin dependencias externas (`go.mod` solo tiene el módulo propio) — CLI router manual en vez de `cobra`/`urfave/cli`.
- Lock propio basado en `O_CREATE|O_EXCL` en vez de `syscall.Flock`, porque Flock no es portable a Windows sin cgo.
- `atomicWriteFile` extraído a `internal/fsutil` desde el primer commit (en vez de duplicarlo luego en `queue`/`settings`) tras notar que Phase 3 y 4 lo necesitarían igual.

**Problemas encontrados y corregidos:** bug inicial en `cli.Run` — los flags globales antes del subcomando (`iameter --config-dir X status`) no se reconocían porque `Run` asumía `args[0]` como comando. Corregido con `splitArgs`, verificado con prueba manual.

**Riesgos pendientes:** ninguno nuevo respecto a Fase 0. Los checks de `doctor` para Claude Code/statusLine/cola/credenciales/daemon quedan explícitamente como WARN "not yet implemented" hasta que esas fases se completen — no se ocultan.

**Siguiente fase:** Fase 2 — captura statusLine y parser del proveedor Claude Code.

## Phase 2 — Captura statusLine — `[x]`
Criterios: parser cumple lista blanca estricta; 17 casos de prueba de sección 26 "Parser" cubiertos; fixtures de sección 27 creados; sin red obligatoria; ausencia ≠ cero.

**Archivos creados:** `internal/providers/provider.go` (interfaz `UsageProvider`), `internal/providers/claude/{claude,claude_test}.go`, `internal/capture/{capture,capture_test}.go`, `internal/statusline/{render,render_test}.go`, `internal/cli/statusline_cmd.go`, 6 fixtures en `testdata/statusline/`.

**Funcionalidad implementada:**
- `providers.UsageProvider{Name() string; Parse(io.Reader) (*model.RateLimits, error)}` — interfaz de la sección 7. `claude.Provider` es la única implementación; la lista blanca se aplica **a nivel de tipo**: `rawEnvelope` solo declara `rate_limits`, así que `encoding/json` descarta automáticamente cualquier otro campo (model, workspace, cost, session_id, transcript_path, git, cwd, env, etc.) — no hay filtrado posterior que se pueda olvidar.
- Ventana inválida (porcentaje fuera de 0–100, tipo incorrecto, `resets_at` nulo/inválido/negativo) se descarta silenciosamente (`nil`, ventana ausente) en vez de fallar todo el parseo o inventar un valor — 0% se preserva como válido, nunca se confunde con ausencia.
- `internal/capture.ReadLimited`: límite de 1 MiB antes de tocar el parser.
- `internal/statusline.Render`: los 4 formatos exactos de la sección 12.
- `iameter statusline`: stdin → capture → parser → render → stdout. **Nunca** bloquea por red, **siempre** imprime algo y sale con código 0 incluso ante JSON malformado/vacío/gigante (degrada a "Consumo no disponible" y registra el motivo en stderr — nunca el JSON crudo). Genera y persiste `device_id` en el primer uso si no existe (necesario para el snapshot antes de emparejar).

**Pruebas ejecutadas:** `go test ./...` (parser: 17/17 casos de la sección 26 + fixtures de la sección 27, incluida prueba de concurrencia con 50 goroutines; capture: límite exacto/por encima/vacío; statusline: los 4 formatos + 0% explícito). `go vet ./...`, `gofmt -l .` limpios. Prueba manual end-to-end con los 6 fixtures reales vía binario compilado, incluida una entrada de 1.1 MB (rechazada) y 20 invocaciones concurrentes contra el mismo `device.json` (sin corrupción, JSON válido tras la carrera, verificado con `python3 -c "json.load(...)"`). Latencia medida: ~2 ms.

**Decisiones técnicas:**
- `resets_at: null` o inválido descarta la ventana completa (no solo el campo) — un timestamp de reinicio inventado sería tan engañoso como un porcentaje inventado, aunque el prompt solo lo exige explícitamente para porcentajes.
- El comando `statusline` nunca retorna código de salida distinto de 0 por fallos de parseo/captura, para no romper la barra de estado de Claude Code; los errores solo van a stderr.

**Problemas encontrados:** ninguno bloqueante. Riesgo menor identificado y aceptado: en una carrera de *primer arranque* (dos invocaciones concurrentes sin `device.json` previo), ambas generan un `device_id` distinto y la última escritura atómica gana — el archivo nunca se corrompe, pero el id "definitivo" es no determinista en ese instante único. No afecta a snapshots ya emparejados (Fase 5 adopta el `device_id` del backend). Documentado, no bloquea aceptación.

**Siguiente fase:** Fase 3 — integración con `settings.json` de Claude Code (instalación, backup, encadenamiento de statusLine previo).

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
