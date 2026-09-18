# Secure File Transfer Node (`sftnode`)

Transferencia segura de archivos entre las sedes de INNOVATECH SOLUTIONS S.A.S.
Escrito en Go, sobre SSH/SFTP, para infraestructura on-premise Linux.

Se indican los archivos a enviar y la carpeta donde guardarlos. Cada archivo se
codifica, cifra, verifica, comprime y se transmite por un canal SSH. Junto al
paquete viaja el propio programa, de modo que la sede destino siempre puede
restaurarlo.

---

## Puesta en marcha

Todos los comandos se ejecutan en **PowerShell**, desde la raíz del proyecto.

```powershell
cd C:\Users\Nicolas\Documents\git\secure-file-transfer-node
```

### 1. Compilar

```powershell
.\scripts\build.ps1
```

Genera los dos binarios que hacen falta:

| Archivo | Para qué |
|---|---|
| `bin\sftnode.exe` | cliente que se ejecuta en Windows |
| `bin\sftnode` | binario Linux que viaja a la sede para restaurar |

Si falta el segundo, el envío avisa en amarillo y continúa, pero la sede se
queda sin el programa para recuperar los archivos.

### 2. Levantar la sede de prueba

```powershell
docker build -t sede-linux -f deployments/Dockerfile.sede deployments
docker run -d --name sede_mexico -p 2201:22 sede-linux
```

Si ya existe y solo está detenida:

```powershell
docker start sede_mexico
```

Para entrar a mirar (contraseña `sede123`):

```powershell
ssh sede@localhost -p 2201
```

### 3. Definir las claves

Se pierden al cerrar la terminal, así que hay que repetirlas en cada sesión.

```powershell
$env:SFT_PASSPHRASE   = "ClaveDeCifrado2026"
$env:SFT_SSH_PASSWORD = "sede123"
```

| Variable | Protege | Valor |
|---|---|---|
| `SFT_PASSPHRASE` | el contenido del archivo | la elige usted |
| `SFT_SSH_PASSWORD` | el canal SSH | `sede123`, definido en el Dockerfile |

Son dos cosas distintas a propósito: aunque alguien intercepte el paquete o
consiga la credencial SSH, sin la passphrase no puede leer el contenido.

### 4. Enviar

```powershell
.\bin\sftnode.exe send `
  --url sftp://sede@localhost:2201 `
  --file "tmp\archivo_x.txt" `
  --dest /home/sede `
  --insecure-host-key
```

Varios archivos, repitiendo `--file` o con comodines:

```powershell
.\bin\sftnode.exe send `
  --url sftp://sede@localhost:2201 `
  --file "tmp\Desarrollo-23-02-2026.CiudadMexico" `
  --file "tmp\Comercial-*.Santiago" `
  --dest /home/sede `
  --insecure-host-key
```

Se envían **solo los archivos indicados**. Si alguno no existe, el comando se
detiene antes de transmitir nada.

### 5. Ver qué llegó

```powershell
docker exec -u sede sede_mexico ls -lh /home/sede
```

```
-rw-r--r--  549   archivo_x.txt.zip     <- el paquete
-rwxr-xr-x  7.7M  sftnode               <- el programa que viajó con él
```

### 6. Restaurar en la sede

Se usa el binario que acaba de llegar, indicando qué paquetes recuperar:

```powershell
docker exec -e SFT_PASSPHRASE=ClaveDeCifrado2026 -u sede -w /home/sede sede_mexico `
  ./sftnode receive --file /home/sede/archivo_x.txt.zip --dest /home/sede
```

Varios a la vez:

```powershell
docker exec -e SFT_PASSPHRASE=ClaveDeCifrado2026 -u sede -w /home/sede sede_mexico `
  ./sftnode receive `
    --file /home/sede/Desarrollo-23-02-2026.CiudadMexico.zip `
    --file /home/sede/Comercial-10-11-2026.Santiago.zip `
    --dest /home/sede
```

La `SFT_PASSPHRASE` debe ser la misma con la que se envió.

---

## Opciones

### `send`

| Opción | Obligatoria | Efecto |
|---|---|---|
| `--url` | sí | servidor `sftp://usuario@host:puerto` |
| `--file` | sí | archivo a enviar; se puede repetir o usar comodines |
| `--dest` | no | carpeta remota donde guardar los paquetes |
| `--programa` | no | binario Linux que viaja con el paquete (`bin\sftnode`) |
| `--sin-programa` | no | no enviar el binario a la sede destino |
| `--key-file` | no | clave privada SSH (`~/.ssh/id_ed25519`) |
| `--known-hosts` | no | host keys conocidas (`~/.ssh/known_hosts`) |
| `--dry-run` | no | valida nombres y parámetros sin cifrar ni conectarse |
| `--strict-naming` | no | rechaza los nombres que no cumplen la nomenclatura |
| `--insecure-host-key` | no | omite la verificación del servidor (solo laboratorio) |
| `--no-color` | no | salida sin color |

### `receive`

| Opción | Obligatoria | Efecto |
|---|---|---|
| `--file` | sí | paquete `.zip` a restaurar; se puede repetir |
| `--dest` | no | carpeta donde dejar los archivos originales |
| `--no-color` | no | salida sin color |

### Carpeta destino

Se toma de `--dest` o de la carpeta incluida en `--url` (`--dest` tiene
prioridad). **Debe existir ya en el servidor**: el programa no crea carpetas
ajenas. Si no existe, se avisa en amarillo y se usa la primera de estas que
funcione, creándola si hace falta:

| Sistema | Carpeta |
|---|---|
| Linux | `/home/<usuario>/sftnode` |
| Linux | `/tmp/sftnode` |
| Windows | `C:/Users/<usuario>/sftnode` |

Para usar una carpeta propia, créela antes en el servidor:

```powershell
docker exec -u sede sede_mexico mkdir -p /home/sede/incoming
```

En `receive`, si `--dest` no existe se usa la carpeta temporal del sistema:
`/tmp/sftnode-restaurados` en Linux, `%TEMP%\sftnode-restaurados` en Windows.

### El programa viaja con el paquete

En cada envío se copia también el binario Linux a la carpeta destino, con
permiso de ejecución y reemplazando el de envíos anteriores. Así la sede nunca
depende de una instalación previa ni de una versión desactualizada.

Se toma de `--programa`, que por defecto es el archivo `sftnode` que esté junto
a `sftnode.exe`. Si no se encuentra, se avisa en amarillo con el comando para
generarlo y los archivos se envían igual. Con `--sin-programa` no se envía.

### Colores del log

Verde: correcto · Amarillo: alerta recuperable · Rojo: error, se aborta ese
archivo · Gris: informativo.

### Códigos de salida

`0` correcto · `1` algún archivo falló · `2` error de configuración ·
`3` no se pudo conectar

---

## Cómo funciona

Cada archivo recorre seis etapas antes de quedar guardado en la sede destino:

| # | Etapa | Mecanismo | Garantiza |
|---|---|---|---|
| 1 | Codificación | Base64 | transporte uniforme del contenido binario |
| 2 | Cifrado | AES-256-GCM (clave derivada con Argon2id) | confidencialidad |
| 3 | Integridad | SHA-256 + etiqueta GCM | integridad |
| 4 | Compresión | ZIP (archivo cifrado + manifiesto) | eficiencia del canal |
| 5 | Transmisión | SFTP dentro de un canal SSH | confidencialidad en tránsito |
| 6 | Verificación | relectura del paquete y comparación del hash | entrega comprobada |

```
Bogotá (cliente)                            Sede destino (servidor)
  archivo original
      | Base64
      | AES-256-GCM      ──── canal SSH cifrado ────►   carpeta destino
      | SHA-256                   (SFTP)                   paquete .zip
      | ZIP                                                + sftnode
                                                                |
                                                        receive: verifica
                                                        hash, descifra y
                                                        restaura el original
```

El ZIP transmitido contiene el archivo cifrado (`.enc`) y un `manifiesto.json`
con el área, la fecha, la sede y el hash SHA-256.

### Nomenclatura

`Area-DD-MM-AAAA.Sede`, por ejemplo `Desarrollo-23-02-2026.CiudadMexico`.
Un nombre que no la cumpla genera una alerta amarilla y se envía igualmente,
salvo que se use `--strict-naming`.

### Estructura del código

| Ruta | Responsabilidad |
|---|---|
| `cmd/sftnode/` | punto de entrada del binario |
| `internal/cli/` | subcomandos `send` y `receive`, códigos de salida |
| `internal/config/` | flags, valores por defecto y lectura de la URL |
| `internal/naming/` | nomenclatura `Area-DD-MM-AAAA.Sede` |
| `internal/paquete/` | Base64, AES-256-GCM, SHA-256, ZIP y manifiesto |
| `internal/envio/` | conexión SSH y transferencia SFTP |
| `internal/logx/` | registro en consola con color |
| `deployments/` | imagen Docker de una sede receptora |
| `scripts/` | compilación de los dos binarios |

---

## Notas de seguridad

- Las claves nunca se pasan por la línea de comandos: se leen de
  `SFT_PASSPHRASE` y `SFT_SSH_PASSWORD`, porque los argumentos de un proceso
  son visibles para todo el sistema.
- La contraseña se convierte en clave AES con Argon2id y sal aleatoria por
  archivo; el nonce de GCM también es único en cada cifrado.
- La host key del servidor se contrasta con `known_hosts`;
  `--insecure-host-key` desactiva esa protección y solo debe usarse en pruebas.
  Para quitarlo, conéctese una vez con `ssh sede@localhost -p 2201` y acepte el
  fingerprint: a partir de ahí el binario verifica el servidor por su cuenta.
- El paquete se sube como `.part` y se renombra al terminar, de modo que la
  sede destino nunca procese un archivo incompleto.
- Al restaurar, un paquete alterado o una clave equivocada se rechazan: el ZIP
  no abre, el hash no coincide o la etiqueta GCM falla.

## Problemas frecuentes

| Síntoma | Causa |
|---|---|
| `falta la clave de cifrado` | no se definió `SFT_PASSPHRASE` en esta terminal |
| `clave incorrecta o datos alterados` | la clave del `receive` no es la del `send` |
| `No existe el binario Linux ...` | falta `bin\sftnode`; ejecute `.\scripts\build.ps1` |
| `La carpeta ... no existe en el servidor` | créela antes, o deje que use la de respaldo |
| `no se pudo conectar` | el contenedor está detenido (`docker start sede_mexico`) |
| rutas convertidas a `C:/Program Files/Git/...` | se ejecutó en Git Bash; use PowerShell |
| los archivos de la sede desaparecieron | el contenedor se recreó; `docker run` sin volúmenes no conserva datos |
