# Alcance objetivo de la plataforma

## Objetivo actual

La plataforma existe para dos flujos y dos públicos:

1. **Registro de lista por parte de los docentes**: pasar lista,
   registrar llamados de atención e incidentes, corregir marcas.
2. **Análisis por parte de los admins**: consola de administración,
   estadísticas por salón y estudiante, cierres de año, secretaría
   como apoyo con permisos reducidos.

Todo lo que se diseñe, se pruebe y se despliegue apunta a esos dos
flujos. Los criterios de aceptación de las historias de usuario se
leen bajo este alcance.

## No objetivo (tardío): distribución a estudiantes

La distribución de la plataforma a los estudiantes —cuentas masivas,
acceso general, publicación del APK a estudiantes, soporte a ese
público— **no es objetivo ahora mismo**. Se considera algo tardío:
podrá retomarse cuando el registro docente y el análisis admin estén
consolidados en producción.

### La pantalla de estudiante existe pero no es producto

Hay una implementación básica de *Mi historial* (`/estudiante`:
ficha propia, asistencia, llamados y faltas en lectura) porque el
backend ya autorizaba "own history" y serviría de referencia técnica
futura. **No debe interpretarse como un lanzamiento**:

- No crear cuentas de estudiante masivamente en producción.
- No distribuir ni anunciar el acceso a estudiantes.
- No comprometer soporte, rendimiento ni diseño para ese público.
- La cuenta demo `estudiante / estudiante123*` es solo para
  verificación técnica del flujo de lectura propia.

Si una decisión de diseño o de despliegue choca con el uso
estudiantil, gana el alcance objetivo: registro docente y análisis
admin.
