# Endurecimiento de seguridad (sondeo externo)

Resultado de una revisión **no autenticada y de solo lectura** contra el
API público (`https://api.fractalschool.app`, versión `v0.1.4`), hecha el
2026-11-07 con autorización del responsable del proyecto. No se usaron
credenciales, no se hizo fuerza bruta, no se enviaron escrituras válidas
y no se leyó la base de datos.

El objetivo era responder una pregunta concreta: **¿qué ve un usuario no
autorizado?** La respuesta corta es que no se encontró exposición de
datos. Lo que sigue documenta la evidencia, los pocos puntos débiles
observados y cómo corregirlos.

## Método

- Peticiones `GET`/`POST`/`PUT`/`DELETE`/`OPTIONS` sin token, con UUID
  aleatorios y cuerpos inválidos (para que, si fallara el auth, la
  validación impidiera escribir).
- Un JWT forjado con `alg: none` y rol `owner`.
- Identidad de desarrollo (`X-Subject-ID`) y `Authorization` basura o
  `Basic`.
- Revisión del código del backend para confirmar la causa de cada
  comportamiento observado.
- Comparación de tiempos de login en varias muestras, más inspección
  del flujo `Authenticate`.

No se probó: separación de permisos con usuarios reales, login exitoso,
entrega real de correos de recuperación, ni la base de datos.

## Resumen

| # | Hallazgo | Severidad | Estado |
|---|---|---|---|
| 1 | HTTP sin redirección a HTTPS y sin HSTS | Media | HSTS en código; falta **Always Use HTTPS** en Cloudflare |
| 2 | Cookie de refresh sin `Secure` detrás del túnel | Media | **Corregido** (`SecureCookies` en producción) |
| 3 | Enumeración de usuarios por tiempo de respuesta | Baja–Media | **Corregido** (bcrypt de descarte; correo en segundo plano) |
| 4 | Throttle por identificador+IP, sin límite global por IP | Media | **Corregido** (60/min por IP) |
| 5 | `TRUSTED_PROXIES` sin verificar en producción | Media si está vacío | Aviso al arrancar; falta confirmar el valor en la instancia |
| 6 | Cabeceras de endurecimiento ausentes y versión expuesta | Baja | **Corregido** salvo `/healthz` (la SPA usa `version`) |
| 7 | `X-Forwarded-For` tomado por la izquierda (falsificable) | Alta al fijar `TRUSTED_PROXIES` | **Corregido** (se toma el salto confiable más a la derecha) |

## Lo que está bien (verificado)

Estos controles pasaron y conviene no romperlos al tocar auth:

- **Todo lo protegido responde `401`.** Más de 40 rutas sin token,
  incluidas docentes, estudiantes, usuarios, materias, horarios,
  auditoría, estadísticas y fotos, devolvieron
  `401 http.err_unauthorized` con el mismo cuerpo.
- **La identidad de desarrollo está apagada.** `X-Subject-ID` en
  `/v1/me`, `/v1/teachers` y `/v1/users` → `401`. Coincide con
  `AllowDevIdentity: !cfg.Env.IsProduction()` en
  `src/cmd/server/main.go`.
- **Tokens forjados rechazados.** `alg: none` → `401
  auth.err_invalid_token`; `Bearer basura` y `Basic` → `401`.
- **Login genérico.** Usuario inexistente y cadena tipo SQLi devuelven
  el mismo `401 users.err_invalid_credentials`, sin errores `500` ni
  trazas de Go.
- **Recuperación sin filtración.** Identificador inexistente → `204`;
  token falso en verify/reset → `401` genérico.
- **Sin superficie de información.** `/.env`, `/openapi.json`, `/docs`,
  `/swagger`, `/metrics`, `/debug/pprof/`, `/config` y `/backup.sql`
  → `404` sin contenido.
- **CORS con lista blanca.** Solo `https://fractalschool.app` recibe
  `Access-Control-Allow-Origin`; un origen ajeno no recibe cabecera.
- **TLS correcto.** TLS 1.3, certificado válido con SAN
  `fractalschool.app` y `*.fractalschool.app` (Google Trust Services).
- **Origen oculto.** `Server: cloudflare`, sin `X-Powered-By` y sin
  trazas del runtime.

## Hallazgo 1 — HTTP sin redirección y sin HSTS

**Severidad:** media.

Evidencia:

```sh
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  http://api.fractalschool.app/healthz
# 200 (sin redirect_url)

curl -sS -D - -o /dev/null https://api.fractalschool.app/healthz \
  | grep -i strict-transport-security
# (sin cabecera)
```

`http://api.fractalschool.app/healthz` y
`http://api.fractalschool.app/v1/teachers` responden `200`/`401` por
HTTP plano en lugar de redirigir a HTTPS. Las respuestas HTTPS tampoco
traen `Strict-Transport-Security`.

**Riesgo:** un cliente que use `http://` (enlace viejo, SPA mal
configurada, atacante en la red) envía credenciales o tokens por un
canal sin cifrar. El túnel cifra Cloudflare→origen, pero no el tramo
navegador→Cloudflare.

**Remediación (Cloudflare, sin tocar código):**

1. Regla **Always Use HTTPS** para `api.fractalschool.app` (y el resto
   de la zona).
2. Cabecera HSTS en la zona o en una Transform Rule:
   `Strict-Transport-Security: max-age=31536000; includeSubDomains`.
   No la actives antes de confirmar que todo el dominio sirve HTTPS:
   HSTS se cachea en el navegador y es difícil de revertir.
3. Opcional: `max-age=63072000; includeSubDomains; preload` solo si el
   dominio entra en la lista de precarga.

## Hallazgo 2 — Cookie de refresh sin `Secure` tras el túnel

**Severidad:** media.

El código decide `Secure` según la conexión que ve el proceso Go:

```go
// src/platform/http/login.go:142
func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/v1",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil, // ← false detrás de Cloudflare Tunnel
		SameSite: http.SameSiteLaxMode,
	})
}
```

Cloudflare Tunnel termina TLS y habla HTTP con `127.0.0.1:8080`, así
que `r.TLS` es `nil` en producción y la cookie sale sin `Secure`. No se
pudo confirmar con un login válido (faltan credenciales), pero el
`Set-Cookie` de logout —que usa `clearRefreshCookie`, sin `Secure`—
muestra el mismo patrón:

```
set-cookie: refresh_token=; Path=/v1; Max-Age=0; HttpOnly; SameSite=Lax
```

**Riesgo:** sin `Secure`, el navegador puede enviar la cookie por una
petición `http://` si alguna vez existe ese camino. `HttpOnly` y
`SameSite=Lax` siguen protegiendo contra XSS y CSRF, por eso la
severidad no es alta.

**Remediación:** no derivar `Secure` de `r.TLS` en producción.
Opciones, en orden de preferencia:

1. Campo de configuración explícito, derivado de `APP_ENV`:
   `Secure: cfg.SecureCookies` con `cfg.SecureCookies = cfg.Env.IsProduction()`.
2. Si se quiere soportar un proxy delante de un origen HTTP, leer
   `X-Forwarded-Proto` **solo** cuando `RemoteAddr` esté en
   `TRUSTED_PROXIES`. Nunca confiar en la cabecera sin lista blanca.
3. Aplicar lo mismo a `clearRefreshCookie` (`src/platform/http/login.go`),
   que hoy ni siquiera recibe `r`.

Añadir una prueba en `src/tests/http/login_test.go` que verifique
`Secure` en el `Set-Cookie` cuando el entorno es producción.

## Hallazgo 3 — Enumeración de usuarios por tiempo

**Severidad:** baja–media.

`Authenticate` retorna antes de `bcrypt` cuando el usuario no existe:

```go
// src/core/users/service.go:159
u, err = s.store.ByUsername(ctx, normalizeUsername(identifier))
if err != nil {
	return User{}, ErrInvalidCredentials // ← sin bcrypt
}
...
if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
	return User{}, ErrInvalidCredentials
}
```

El mismo patrón está en `RecoveryService.Request`
(`src/core/users/recovery.go:160`), donde el retorno temprano además
evita generar y guardar el token.

En una primera muestra se observó una diferencia de ~120 ms entre una
cuenta existente y un usuario aleatorio. En una segunda tanda de 5
muestras la diferencia **no se reprodujo**: el ruido de Cloudflare
(conexión, colas, caché de TLS) domina la señal y los tiempos quedaron
en ~0.6–0.8 s en ambos casos. Por eso se reporta como **riesgo latente
confirmado en código, no demostrado de forma estable por red**.

**Riesgo:** si se mide con suficiente precisión, permite confirmar si
un nombre de usuario o correo existe aunque el mensaje sea genérico.
Combinado con el hallazgo 4, la enumeración se vuelve práctica.

**Remediación:** ejecutar siempre un `bcrypt` contra un hash dummy
cuando la cuenta no exista o esté inactiva:

```go
// hash constante de una contraseña aleatoria, generado al arrancar
var dummyHash = mustHash(randomPassword())

if err != nil {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
	return User{}, ErrInvalidCredentials
}
```

Hacer lo mismo en `RecoveryService.Request` antes de devolver `"", nil`.
La meta es que la rama "no existe" cueste lo mismo que "existe y la
contraseña está mal".

## Hallazgo 4 — Límite por identificador+IP, sin límite global por IP

**Severidad:** media.

```go
// src/platform/http/login.go:130
func loginKey(l *loginLimiter, r *http.Request, identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier)) + "|" + l.clientIP(r)
}
```

El presupuesto es `10/min` por **identificador + IP**
(`loginLimitPerMinute`, `src/platform/http/login.go:65`). Un atacante
con una sola IP puede probar 10 contraseñas por cada nombre de usuario
distinto, sin límite agregado. Con el hallazgo 3 puede además enumerar
cuentas primero.

**Riesgo:** fuerza bruta distribuida por nombres de usuario y
enumeración práctica. El coste por cuenta sigue acotado, pero no hay
techo global.

**Remediación:** registrar dos claves por intento:

- `login|<identificador>|<ip>` — la actual, para proteger una cuenta.
- `login|<ip>` — límite agregado más alto (p. ej. `60/min`) para
  frenar el barrido de identificadores.

Mantener el límite por cuenta como está y no contar los intentos
denegados (el comportamiento actual evita extender la ventana). Añadir
pruebas en `src/tests/http/login_test.go` que cubran el límite por IP.

## Hallazgo 5 — Verificar `TRUSTED_PROXIES` en producción

**Severidad:** media **si** está vacío.

```go
// src/core/throttle/throttle.go:144
func ClientIP(remoteAddr, forwardedFor string, trusted *Nets) string {
	...
	if trusted == nil || !trusted.Contains(host) {
		return host // ← 127.0.0.1 detrás del túnel
	}
	...
}
```

Con `TRUSTED_PROXIES` vacío, `clientIP` ignora `X-Forwarded-For` y ve
`127.0.0.1` para **todas** las peticiones que llegan por Cloudflare
Tunnel. Entonces la clave `identificador|127.0.0.1` es global: 10
intentos —fallidos o no— contra una cuenta conocida la bloquean para
todos, y el límite por IP del hallazgo 4 no distinguiría clientes. En un
colegio eso también puede producir `429` legítimos en horas pico.

No se pudo leer `/etc/grado.env` desde fuera. `docs/deploy.md` incluye
`TRUSTED_PROXIES` en la tabla de configuración, pero no confirma el
valor desplegado.

**Remediación:**

1. Confirmar en la instancia:
   `grep '^TRUSTED_PROXIES=' /etc/grado.env`.
2. Si Cloudflare Tunnel entrega desde `127.0.0.1`/`::1`, configurar
   `TRUSTED_PROXIES=127.0.0.1/32,::1/128` (el proxy local). Si en el
   futuro se expone el origen a la red, usar los rangos de Cloudflare.
3. Documentar el valor en `docs/deploy.md` junto al túnel y añadir una
   comprobación de arranque que avise en producción si está vacío.

## Hallazgo 6 — Cabeceras y divulgación menor

**Severidad:** baja.

- Falta `X-Content-Type-Options: nosniff` en las respuestas JSON (solo
  aparece en el 404 por defecto de Go).
- No hay `Cache-Control: no-store` en respuestas de autenticación
  (`/v1/login`, `/v1/refresh`, `/v1/me`), lo que permite que un proxy o
  el navegador cachee material sensible.
- `/healthz` publica `version` y `recoveryMail`:

  ```json
  {"recoveryMail":true,"status":"ok","version":"v0.1.4"}
  ```

  Es información menor, pero revela la versión desplegada a cualquiera.

**Remediación:**

- Middleware con `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: no-referrer` y `X-Frame-Options: DENY` (o
  `frame-ancestors` en CSP) para todas las respuestas.
- `Cache-Control: no-store` en las rutas de auth.
- Si se quiere recortar `/healthz`, dejar `status` para el chequeo y
  mover `version`/`recoveryMail` a un endpoint interno o a logs.

## Hallazgo 7 — `X-Forwarded-For` falsificable

**Severidad:** alta en cuanto se configure `TRUSTED_PROXIES` (encontrado
en revisión de código el 2026-11-12, después del sondeo).

`ClientIP` tomaba la entrada **más a la izquierda** de `X-Forwarded-For`.
Cloudflare *agrega* la IP real al final de lo que mande el cliente, así
que `X-Forwarded-For: 6.6.6.6` llegaba como `6.6.6.6, <ip real>` y el
throttle usaba `6.6.6.6`: cambiando ese valor en cada intento, un
atacante tenía presupuesto infinito de contraseñas.

**Corrección:** recorrer la cabecera desde la derecha y quedarse con la
primera entrada que no sea un proxy confiable
(`src/core/throttle/throttle.go`, prueba
`TestClientIPIgnoresSpoofedForwardedFor`).

## Cómo reproducir el sondeo

Los comandos mínimos, sin credenciales:

```sh
BASE=https://api.fractalschool.app

# Rutas protegidas: todas deben ser 401
for p in /v1/me /v1/teachers /v1/users /v1/students/00000000-0000-4000-8000-000000000000 \
         /v1/audit /v1/schedule /v1/dashboard/summary; do
  printf '%s -> ' "$p"
  curl -sS -o /dev/null -w '%{http_code}\n' "$BASE$p"
done

# Identidad de desarrollo apagada
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H 'X-Subject-ID: 00000000-0000-4000-8000-000000000000' "$BASE/v1/me"

# Sin redirección a HTTPS ni HSTS
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' http://api.fractalschool.app/healthz
curl -sS -D - -o /dev/null "$BASE/healthz" | grep -i strict-transport-security

# CORS: un origen ajeno no debe recibir Access-Control-Allow-Origin
curl -sS -D - -o /dev/null -H 'Origin: https://evil.example' "$BASE/v1/me" \
  | grep -i access-control-allow-origin
```

No ejecutes fuerza bruta ni escrituras: el sondeo se diseñó para no
tocar datos.

## Checklist de despliegue

- [ ] Activar **Always Use HTTPS** en Cloudflare para la zona.
- [x] Añadir HSTS (el API lo envía en producción; el SPA en `_headers`).
- [ ] Fijar `TRUSTED_PROXIES=127.0.0.1/32,::1/128` (o el proxy real).
- [x] Forzar `Secure` en la cookie de refresh en producción.
- [x] Igualar el tiempo de login para cuentas inexistentes.
- [x] Añadir límite de login agregado por IP.
- [x] Añadir `nosniff`, `no-store` y cabeceras de framing.
- [x] Tomar la IP real de `X-Forwarded-For` por la derecha.
- [x] `/healthz` sigue publicando `version`: la SPA la usa para distinguir
  un despliegue de una sesión vencida. Riesgo aceptado.
- [ ] Repetir el sondeo después del cambio y guardar la evidencia.

## Referencias

- `src/platform/http/login.go` — `setRefreshCookie`, `clearRefreshCookie`,
  `loginKey`, límites y ventana.
- `src/core/users/service.go` — `Authenticate` (bcrypt y retorno temprano).
- `src/core/users/recovery.go` — `RecoveryService.Request`.
- `src/core/throttle/throttle.go` — `ClientIP` y `ParseTrusted`.
- `src/platform/config/config.go` — `ALLOWED_ORIGINS`, `TRUSTED_PROXIES`.
- `src/platform/http/routes.go` — rutas públicas/protegidas y permisos.
- `src/platform/http/auth.go` — `MiddlewareSubjectWith`.
- `docs/deploy.md` — túnel, variables de producción y `TRUSTED_PROXIES`.
- `docs/api.md` — contrato de autenticación y autorización.
