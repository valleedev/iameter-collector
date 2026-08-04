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

## Phase 3 — Config Claude Code / instalación local — `[x]`
Criterios: instala/preserva/encadena statusLine previo; backup; idempotente; restaura al desinstalar; soporta rutas con espacios/Unicode; nunca sobrescribe JSON corrupto.

### Verificación contra documentación oficial
Se consultó `https://code.claude.com/docs/en/settings`: confirma que la configuración de usuario vive en `~/.claude/settings.json` (macOS/Linux) y `%USERPROFILE%\.claude\settings.json` (Windows) — exactamente lo implementado.

**Archivos creados:** `internal/settings/{locate,settings,command,install,uninstall,install_test,uninstall_test}.go`, `internal/statusline/{chain,chain_unix,chain_windows,chain_test}.go`, `internal/cli/{install_cmd,uninstall_cmd}.go`. `statusline_cmd.go` extendido para encadenar.

**Funcionalidad implementada:**
- `settings.Install`/`settings.Uninstall`: backup automático (una sola vez, nunca se sobrescribe el snapshot original), preserva todas las claves ajenas del JSON (map genérico `map[string]json.RawMessage`), detecta 4 casos (`ausente` / `ya es IA METER` / `binario movido` / `externo → encadenar`), nunca sobrescribe JSON corrupto (`ErrCorruptJSON`), rechaza symlinks (`ErrSymlink`, sección 24).
- Encadenamiento real: `iameter statusline` ejecuta el comando externo preservado (vía `/bin/sh -c` o `cmd /C`), pasándole el mismo stdin, e imprime exactamente su salida — mientras parsea el consumo en paralelo (goroutines). Si el comando encadenado falla, hace fallback silencioso al render propio (nunca rompe la barra de estado).
- `BuildCommand` cita la ruta del binario por plataforma (comillas simples POSIX / dobles Windows) para soportar espacios y Unicode (sección 31) — verificado con ruta real `/tmp/iameter fake höme/config with spaces`.
- `iameter install`/`iameter uninstall` reales: detectan Claude Code (`~/.claude` o `claude` en PATH), configuran/restauran el statusLine, generan `device_id`, informan qué falta (emparejamiento Fase 5, daemon Fase 6) sin fingir que existe.
- `doctor` ahora reporta de verdad "Claude Code detection" y "StatusLine configuration" (antes eran WARN "not yet implemented").

**Pruebas ejecutadas:** `go test ./...` — 16 pruebas nuevas en `internal/settings` cubriendo exactamente la lista de la sección 26 "Configuración" (archivo inexistente, válido, statusLine ausente/de IA METER/externo, JSON inválido, instalación repetida, restauración, desinstalación, prevención de recursión, symlink) + 5 en `internal/statusline` para el encadenamiento. `go vet`, `gofmt -l .` limpios. Prueba manual end-to-end con el binario real: instalar sobre un `settings.json` con statusLine externo simulando Starship, verificar encadenamiento real (`echo external-tool-output` se ejecuta y su salida se imprime), instalar de nuevo (idempotente), desinstalar (restaura exactamente el comando externo original), desinstalar de nuevo (no-op seguro) — todo bajo una ruta `$HOME` con espacios y diéresis (`fake höme`) para validar la sección 31. Cross-compilación verificada de nuevo para los 6 targets tras estos cambios.

**Decisiones técnicas:**
- El estado de encadenamiento (`chained_statusline.json`) se guarda en el `config-dir` propio de IA METER, no junto a `settings.json` de Claude Code, para mantener el estado autocontenido.
- `isIAMeterCommand` usa una heurística (contiene el nombre del binario + termina en "statusline") en vez de comparación exacta de ruta, para seguir detectando una instalación propia aunque el binario se haya movido — limitación aceptada: si el usuario configura manualmente un comando ajeno que también termina en la palabra "statusline" y contiene "iameter" en la ruta, podría clasificarse erróneamente como propio. Caso de borde documentado, no bloqueante.
- **Bug real encontrado y corregido durante las pruebas:** `RunChained` con un comando colgado (`sh -c "sleep 30"`) no respetaba `ChainTimeout` de 3s — tardaba los 30s completos porque el proceso `sleep` (nieto, huérfano tras matar el shell) mantenía abierto el pipe de stdout, bloqueando `cmd.Run()`. Corregido con grupos de proceso (`Setpgid` + `kill(-pid)` en Unix) y `cmd.WaitDelay` como respaldo multiplataforma. Verificado que no queden procesos `sleep` huérfanos tras el fix.

**Riesgos pendientes:** ninguno nuevo. La detección de Claude Code es best-effort (no bloquea instalación si no se encuentra).

**Siguiente fase:** Fase 4 — cola local persistente y funcionamiento offline.

## Phase 4 — Cola local / offline — `[x]`
Criterios: escritura atómica; dedup; recuperación ante corrupción; límite de tamaño; compactación; concurrente-seguro.

**Archivos creados:** `internal/idgen/idgen.go` (helper de IDs extraído de `device` para reutilizar en `queue`), `internal/queue/{queue,queue_test}.go`. `internal/cli/statusline_cmd.go` extendido para persistir; `status_cmd.go`/`doctor_cmd.go` extendidos con datos reales de cola.

**Funcionalidad implementada:**
- `queue.Queue` respaldada por un único `queue.json` (array de `Item`), reescrito atómicamente (`fsutil.AtomicWriteFile`) en cada operación mutante bajo `fsutil.FileLock` — la reescritura completa hace que la "compactación" sea inherente a cada escritura, no un paso aparte (decisión documentada en el código).
- Deduplicación: un snapshot idéntico al último (mismos `used_percentage`/`resets_at` en ambas ventanas) se descarta salvo que hayan pasado ≥5 min (`MinReheartbeat`), en cuyo caso se guarda como heartbeat.
- Límite de tamaño: `MaxItems=500`, recorte por el extremo más antiguo — el snapshot más reciente sobrevive siempre.
- Recuperación ante corrupción: JSON inválido se pone en cuarentena (`queue.json.corrupt-<timestamp>`, nunca se borra) y la cola continúa vacía y operativa.
- `iameter statusline` ahora encola de verdad (objetivo pendiente desde la Fase 2): solo cuando el parseo tuvo éxito y `rate_limits` no está vacío — nunca encola snapshots vacíos ni basura de un parseo fallido.
- `iameter status`/`iameter doctor` leen la cola real: pendientes, último snapshot, porcentajes y horarios de reinicio.

**Pruebas ejecutadas:** `go test ./...` — 14 pruebas nuevas en `internal/queue` (orden FIFO, dedup dentro/fuera de la ventana de heartbeat, cambio de porcentaje siempre encola, `Ack` elimina por ID, `Ack` de ID desconocido es no-op, recorte a `MaxItems` preservando el más reciente, recuperación de archivo corrupto con cuarentena verificable, archivo vacío tratado como cola vacía, 30 goroutines escribiendo concurrentemente sin pérdidas y con JSON válido al final, `Peek` no elimina, `IncrementAttempts` preserva el ID de idempotencia, ciclo de vida completo sin ninguna dependencia de red). `go vet`, `gofmt -l .` limpios. Prueba manual end-to-end: 3 invocaciones de `statusline` (2 idénticas colapsadas por dedup, 1 sin `rate_limits` correctamente no encolada) seguidas de `status`/`doctor` mostrando los datos reales de la cola. Cross-compilación verificada de nuevo para los 6 targets.

**Decisiones técnicas:**
- Formato de archivo único (array JSON) en vez de JSONL — dado que toda operación ya requiere el lock exclusivo (para dedup/recorte/compactación), un JSONL de solo-append no aportaría ninguna ventaja real y complicaría la recuperación ante corrupción (una línea rota vs. un archivo entero roto).
- `MinReheartbeat=5min` y `MaxItems=500` son valores razonables sin requisito numérico explícito en el prompt; documentados como configurables si el uso real lo exige.

**Problemas encontrados:** ninguno bloqueante.

**Riesgos pendientes:** ninguno nuevo. La cola no tiene aún consumidor real (eso es la Fase 5/6: `syncer`/`daemon` llamarán a `Peek`/`Ack`/`IncrementAttempts`).

**Siguiente fase:** Fase 5 — emparejamiento y cliente HTTP contra un mock server de desarrollo.

## Phase 5 — Emparejamiento y backend — `[x]`
Criterios: pairing contra mock server; credenciales nunca en texto plano en logs; cliente HTTP con reintentos/backoff/timeouts/idempotencia; pruebas httptest de todos los códigos de sección 26.

**Archivos creados:** `internal/idgen/{idgen,idgen_test}.go` (extraído de `device`), `internal/credentials/{store,fallback,store_linux,store_darwin,store_windows,fallback_test}.go`, `internal/httpclient/{client,client_test}.go`, `internal/pairing/{pairing,pairing_test}.go`, `internal/syncer/{syncer,syncer_test}.go`, `internal/mockserver/{server,server_test}.go`, `internal/cli/{pair_cmd,unpair_cmd,sync_cmd,mockserver_cmd,cli_test}.go`, `internal/config/{last_snapshot,last_snapshot_test}.go`.

**Funcionalidad implementada:**
- `credentials.Store` (sección 18): Linux vía `secret-tool` (D-Bus Secret Service) con detección de disponibilidad real (prueba un lookup contra el daemon antes de comprometerse); macOS vía CLI `security` (Keychain); Windows vía DPAPI puro (`CryptProtectData`/`CryptUnprotectData` con `syscall.NewLazyDLL`, **sin CGO ni dependencias externas**); fallback a archivo `0600` cuando ninguno está disponible, con `IsFallback()` para que `doctor`/`pair` adviertan al usuario (verificado en este entorno: sin `secret-tool` instalado, cae a fallback correctamente).
- `httpclient.Client`: timeout fijo por request, límite de tamaño de respuesta (1 MiB), `User-Agent: IAMeter-Collector/<version>`, parseo de `Retry-After` (formato segundos y HTTP-date), sin reintentos propios (política de reintentos vive en `syncer`/futuro `daemon`).
- `pairing.Pair`: `POST /v1/devices/pair` con manejo explícito de 400/404/409/403/5xx/JSON inválido/respuesta incompleta — cada uno con un sentinel error propio.
- `syncer.SyncOnce`: recorre la cola en orden FIFO, se detiene en el primer ítem no confirmado; 200/201 confirman y hacen `Ack`; 409 se trata como ya-entregado (replay idempotente); 401/403 detienen todo el lote y devuelven `ErrUnauthorized`; 429/5xx incrementan `Attempts` y detienen el lote respetando `Retry-After`; nunca reordena ni salta ítems.
- `mockserver.Server`: backend de desarrollo real (no simulado) con estado en memoria — códigos de emparejamiento de un solo uso, tokens por dispositivo, deduplicación por `Idempotency-Key`. Expuesto como `iameter mock-server` (comando adicional fuera de los 10 principales, documentado como herramienta de desarrollo).
- `iameter pair/unpair/sync` reales: `pair` rechaza re-emparejar localmente sin `unpair` antes; `sync` reporta claramente "no emparejado" vs. token rechazado vs. error de red; `unpair` borra el token del almacén de credenciales y genera un nuevo `device_id` local.
- **Nuevo:** `config.SaveLastSnapshot`/`LoadLastSnapshot` — caché del último snapshot capturado, independiente del ciclo de vida de la cola, para que `status` siga mostrando el consumo tras una sincronización exitosa (ver "Problemas encontrados").
- `doctor` ahora reporta credenciales y conectividad reales (backend inalcanzable con la URL de desarrollo por defecto es `WARN`, no `ERROR` — es el estado esperado sin `mock-server` corriendo).

**Pruebas ejecutadas:** `go test ./...` — pruebas nuevas: `credentials` (round-trip, ausente, borrado idempotente, permisos 0600, rechazo de path traversal en la clave, selección de backend real vía `New()`); `httpclient` (éxito, `Retry-After` en ambos formatos, error de red, timeout, respuesta sobredimensionada rechazada); `pairing` (éxito + los 6 casos de error de la sección 16, todos con `httptest`); `syncer` (los 11 escenarios de la sección 26 "Sincronización": 200/201/409/401/403/429+Retry-After/5xx/timeout/red/respuesta inesperada/orden/idempotencia entre reintentos/cola vacía/no emparejado); `mockserver` (ciclo completo pair→sync→idempotencia→heartbeat contra el servidor real, no mocks anidados); `cli` (reordenamiento de flags). `go vet`, `gofmt -l .` limpios en Linux/Windows/macOS (build tags). Prueba manual end-to-end repetida dos veces tras corregir bugs reales (ver abajo) con el binario compilado + `iameter mock-server` real: emparejar, re-emparejar (rechazado), capturar vía `statusline`, sincronizar, verificar `status`/`doctor`, inspeccionar el archivo de credenciales fallback, desemparejar, confirmar que `sync` se niega tras desemparejar. Cross-compilación verificada de nuevo para los 6 targets + `go vet` en los 3 SO.

**Decisiones técnicas:**
- Ningún almacén de credenciales usa CGO ni añade dependencias externas: Linux/macOS invocan binarios del sistema (`secret-tool`, `security`) vía `os/exec`; Windows usa únicamente `syscall.NewLazyDLL` (stdlib) para DPAPI.
- `mock-server` es un comando adicional del mismo binario `iameter` (no un binario separado), preservando "un único binario" de la sección 8, y explícitamente fuera de los 10 comandos principales de la sección 10.
- `SyncOnce` no reintenta internamente — cada llamada es una sola pasada; `iameter sync` la invoca una vez y termina (sección 15); el daemon (Fase 6) decidirá cuándo volver a llamarla y con qué backoff.

**Problemas encontrados y corregidos (los tres detectados por las pruebas manuales end-to-end, no por los tests unitarios — quedan documentados porque son la evidencia real de que se ejecutó, no solo se afirmó):**
1. `iameter pair CODE --config-dir X` fallaba: el paquete `flag` de Go deja de reconocer flags tras el primer argumento posicional, así que `--config-dir` y el resto se interpretaban como argumentos posicionales adicionales. Corregido con `reorderFlagsFirst` (mueve flags reconocidas antes que los posicionales), con pruebas dedicadas en `cli_test.go`.
2. `iameter mock-server --pairing-code X` generaba un código pero nunca lo registraba como válido — la variable `X` no se pasaba a `mockserver.New(...)`. Corregido; prueba de regresión `TestMockServerPresetPairingCodeUsable` añadida.
3. Tras una sincronización exitosa, `iameter status` dejaba de mostrar el consumo (leía el último ítem de la cola, que había quedado vacía). Corregido con una caché independiente (`last_snapshot.json`) que sobrevive a la cola.

**Riesgos pendientes:** el heartbeat existe en el mock server y en `syncer.Heartbeat`, pero nada lo invoca todavía — eso es responsabilidad del daemon (Fase 6).

**Siguiente fase:** Fase 6 — daemon multiplataforma (sincronización en segundo plano, heartbeats, backoff exponencial, registro como servicio de usuario en los tres SO).

## Phase 6 — Daemon — `[x]`
Criterios: single-instance; graceful shutdown; heartbeat; backoff+jitter; respeta Retry-After; detiene reintentos en 401/403; `iameter sync` sincroniza una vez y termina.

**Archivos creados:** `internal/daemon/{backoff,daemon,service,service_linux,service_darwin,service_windows}.go` + tests (`backoff_test`, `daemon_test`, `service_linux_test`, `service_darwin_test`, `service_windows_test`), `internal/cli/daemon_cmd.go`. Extendidos: `install_cmd.go` (registro de servicio + `--pair`), `uninstall_cmd.go` (baja de servicio), `pair_cmd.go` (lógica compartida `performPairing`), `doctor_cmd.go`/`status_cmd.go` (estado real del daemon). Eliminado `stubs.go` (ya no quedan comandos pendientes).

**Funcionalidad implementada:**
- `daemon.Run`: single-instance vía `fsutil.FileLock` (reutilizado de la Fase 1/4, con detección de locks obsoletos); apagado limpio por cancelación de contexto (`SIGINT`/`SIGTERM` capturados en el CLI); heartbeat en su propio ticker independiente del ciclo de sync.
- Backoff exponencial con jitter (hasta 20%), respetando `Retry-After` cuando el backend lo indica y es mayor que el backoff calculado.
- Ante `401`/`403` o dispositivo no emparejado: el daemon deja de reintentar sync inmediatamente y pasa a un intervalo de "pausa" largo (por defecto 5 min) que solo revisa si el dispositivo volvió a emparejarse — verificado con test que cuenta requests reales al backend y confirma que no lo bombardea.
- Registro de servicio por SO (sección 20), sin privilegios de administrador en ningún caso:
  - **Linux**: unit `systemd --user` generado y escrito en `$XDG_CONFIG_HOME/systemd/user/iameter.service` (o `~/.config/...`), `systemctl --user enable --now`/`disable --now`. Si `systemctl` no está en PATH, `Install` devuelve un error descriptivo (fallback documentado, sección 20) en vez de fallar silenciosamente o inventar otro mecanismo.
  - **macOS**: `LaunchAgent` (`~/Library/LaunchAgents/com.iameter.collector.plist`) con `RunAtLoad`/`KeepAlive`, `launchctl load/unload -w`.
  - **Windows**: Tarea Programada (`schtasks /Create /SC ONLOGON /RL LIMITED`, sin admin), arrancada inmediatamente tras crearla.
- `iameter daemon`: corre el loop en primer plano (lo que systemd/launchd/schtasks supervisan); `iameter install` ahora registra el servicio y acepta `--pair CODE` para emparejar en el mismo paso; `iameter uninstall` da de baja el servicio.
- `doctor`/`status`: estado real del daemon (`Installed`/`Running`) en vez del `WARN "not yet implemented"` anterior.

**Pruebas ejecutadas:** `go test ./...` — `daemon`: 5 pruebas de backoff/jitter puras + 6 de integración real contra un `httptest.Server` (single-instance lock, apagado limpio, sync exitoso reinicia el intervalo, fallos repetidos aumentan el intervalo, pausa tras 401 sin bombardear el backend — verificado contando requests reales, heartbeat dispara de verdad); generadores de unidad de servicio probados como funciones puras para los 3 SO (contenido del unit systemd, plist de LaunchAgent, comando `schtasks`) — **sin** llamadas reales a `systemctl`/`launchctl`/`schtasks` dentro de la suite automatizada (ver decisión de seguridad abajo). `go vet`/`gofmt -l .`/compilación cruzada verificados en los 3 SO de nuevo.

Prueba manual end-to-end con el binario real y `iameter mock-server`: `iameter daemon` en primer plano (drena la cola sola, sin llamar a `iameter sync`), una segunda instancia concurrente se rechaza con "another instance is already running", apagado limpio verificado con `timeout`+`SIGTERM`. Adicionalmente se verificó el camino de fallo controlado de `install`/`uninstall` cuando `systemctl` no está en el `PATH` (mensaje `WARN` claro, el resto de la instalación continúa).

**Decisión de seguridad importante:** este entorno tiene un `systemctl --user` real y activo (sesión del usuario real de la máquina, no un contenedor desechable). **Deliberadamente no se ejecutó `systemctl --user enable --now`/`launchctl load` contra la sesión real** como parte de esta tarea, porque registrar un servicio persistente con reinicio automático en la máquina real del operador sin que lo haya pedido explícitamente sería una mutación de su entorno fuera del alcance solicitado. Se verificó en su lugar: (a) la generación de contenido es correcta (pruebas unitarias), (b) el camino de error cuando `systemctl` no está disponible funciona limpiamente, (c) `systemctl --user status` (de solo lectura) confirma que el mecanismo existe y sería utilizable. La activación real del servicio en Linux/macOS/Windows queda como verificación manual pendiente para quien despliegue conscientemente (documentado también en limitaciones de la Fase 8).

**Problemas encontrados:** ninguno nuevo en el loop del daemon. Se descubrió y corrigió una inconsistencia de citado en `quoteUnitPath` (solo citaba si detectaba espacios, dejando comillas embebidas sin escapar en rutas sin espacios) — ahora cita siempre, igual que la implementación de Windows.

**Riesgos pendientes:** activación real de systemd/launchd/schtasks no probada end-to-end contra un SO real por la razón de seguridad explicada arriba — solo generación de contenido + manejo de errores verificados.

**Siguiente fase:** Fase 7 — instaladores (`install.sh`/`install.ps1`) y CI/CD (GitHub Actions, compilación de los 6 targets, checksums).

## Phase 7 — Instaladores y distribución — `[ ]`
Criterios: 4 scripts creados; detectan SO/arch; verifican SHA-256; rollback; CI compila 6 targets + checksums.

## Phase 8 — Endurecimiento, documentación, validación final — `[ ]`
Criterios: SECURITY.md, PRIVACY.md completos y verídicos; `go build ./...`, `go vet ./...`, `go test ./...` pasan; 6 binarios compilados; privacidad verificada contra código.

---

## Limitaciones reales conocidas (actualizado en Fase 8)
Ver sección final de este documento, completada al cierre de Fase 8.
