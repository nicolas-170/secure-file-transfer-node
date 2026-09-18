# Secure File Transfer Node (`sftnode`)

Transferencia segura de archivos entre las sedes de INNOVATECH SOLUTIONS S.A.S.
Escrito en Go, sobre SSH/SFTP, para infraestructura on-premise Linux.

Se indican los archivos a enviar y la carpeta donde guardarlos. Cada archivo se
codifica, cifra, verifica, comprime y se transmite por un canal SSH. La sede
destino compila el mismo programa para restaurarlos.

---

# Guía paso a paso

Hay dos máquinas y conviene tener claro en cuál se está en cada momento:

| Máquina | Papel | Terminal |
|---|---|---|
| **Windows** | sede principal de Bogotá, envía | PowerShell |
| **Contenedor Linux** | sede de destino, recibe y restaura | bash, entrando por SSH |

El recorrido completo es: preparar la sede → preparar el cliente → enviar →
comprobar → restaurar. Cada paso indica dónde se escribe.

---

## Paso 1 · Levantar la sede

> En **Windows**, PowerShell, desde la raíz del proyecto.

```powershell
cd C:\Users\Nicolas\Documents\git\secure-file-transfer-node

docker build -t sede-linux -f deployments/Dockerfile.sede deployments
docker run -d --name sede_mexico -p 2201:22 sede-linux
```

La imagen es un Debian con servidor SSH, Go y Git ya instalados. Si el
contenedor ya existe y solo está detenido:

```powershell
docker start sede_mexico
```

Comprobar que responde:

```powershell
docker ps --filter name=sede_mexico
```

---

## Paso 2 · Entrar a la sede

> En **Windows**, PowerShell.

```powershell
ssh sede@localhost -p 2201
```

Contraseña: `sede123`. La primera vez pide aceptar el fingerprint del servidor;
escriba `yes`.

**A partir de aquí y hasta el paso 5, todo se escribe dentro de la sede.**

---

## Paso 3 · Preparar la sede

> Dentro de la **sede**, en bash.

### 3.1 Definir la clave de cifrado

```bash
export SFT_PASSPHRASE="ClaveDeCifrado2026"
```

Eso vale para la sesión actual. Para que quede fija en cada login:

```bash
echo 'export SFT_PASSPHRASE="ClaveDeCifrado2026"' >> ~/.bashrc
source ~/.bashrc
echo $SFT_PASSPHRASE
```

Apunte esa clave: **en el paso 4 hay que usar exactamente la misma en Windows**.
Si difiere, la sede no podrá descifrar lo que reciba.

### 3.2 Descargar el repositorio

`git clone` crea la carpeta con el nombre del repositorio:

```bash
cd ~
git clone https://github.com/<usuario>/secure-file-transfer-node.git
cd secure-file-transfer-node
```

Si el repositorio aún no está publicado, se copia el código desde Windows, en
**otra** terminal de PowerShell, sin cerrar la sesión SSH:

```powershell
docker exec -u sede sede_mexico mkdir -p /home/sede/secure-file-transfer-node
docker cp cmd      sede_mexico:/home/sede/secure-file-transfer-node/
docker cp internal sede_mexico:/home/sede/secure-file-transfer-node/
docker cp go.mod   sede_mexico:/home/sede/secure-file-transfer-node/
docker cp go.sum   sede_mexico:/home/sede/secure-file-transfer-node/
docker exec -u root sede_mexico chown -R sede:sede /home/sede/secure-file-transfer-node
```

### 3.3 Compilar el binario

Desde la raíz del proyecto, el ejecutable se deja en su carpeta `bin/`:

```bash
cd ~/secure-file-transfer-node
go build -o bin/sftnode ./cmd/sftnode
./bin/sftnode help
```

La sede ya está lista para recibir. Deje esta terminal abierta.

---

## Paso 4 · Preparar el cliente

> En **Windows**, en una terminal de PowerShell, desde la raíz del proyecto.

### 4.1 Compilar el cliente

```powershell
cd C:\Users\Nicolas\Documents\git\secure-file-transfer-node
go build -o bin/sftnode.exe ./cmd/sftnode
```

### 4.2 Definir las variables de entorno

```powershell
$env:SFT_PASSPHRASE   = "ClaveDeCifrado2026"
$env:SFT_SSH_PASSWORD = "sede123"
```

| Variable | Protege | Valor |
|---|---|---|
| `SFT_PASSPHRASE` | el contenido del archivo (AES-256) | **la misma del paso 3.1** |
| `SFT_SSH_PASSWORD` | el canal SSH | `sede123`, definido en el Dockerfile |

Son dos cosas distintas a propósito: aunque alguien intercepte el paquete o
consiga la credencial SSH, sin la passphrase no puede leer el contenido.

Estas variables se pierden al cerrar la ventana. Para dejarlas fijas para su
usuario de Windows:

```powershell
[Environment]::SetEnvironmentVariable("SFT_PASSPHRASE", "ClaveDeCifrado2026", "User")
[Environment]::SetEnvironmentVariable("SFT_SSH_PASSWORD", "sede123", "User")
```

Hay que abrir una terminal nueva para que aparezcan. Comprobar:

```powershell
$env:SFT_PASSPHRASE
```

Nunca se pasan por la línea de comandos: los argumentos de un proceso son
visibles para cualquier usuario del sistema.

---

## Paso 5 · Enviar los archivos

> En **Windows**, PowerShell.

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

Qué significa cada parte:

| Parte | Significado |
|---|---|
| `--url` | servidor destino: usuario, host y puerto |
| `--file` | el archivo a enviar; se repite para varios |
| `--dest` | carpeta remota donde dejar el paquete; debe existir |
| `--insecure-host-key` | omite verificar la identidad del servidor, solo en pruebas |

Se envían **solo los archivos indicados**. Si alguno no existe, el comando se
detiene antes de transmitir nada.

La salida muestra las seis etapas y termina en verde si todo fue bien:

```
· Carpeta destino /home/sede
▶ archivo_x.txt  (1-4) Base64 + AES-256-GCM + SHA-256 + ZIP
▶ archivo_x.txt  (5) transmitiendo por SFTP
✔ archivo_x.txt.zip  guardado y verificado en /home/sede
Resumen: 1 enviados · 0 alertas · 0 errores
```

---

## Paso 6 · Comprobar que llegó

> Vuelva a la terminal de la **sede** (la sesión SSH del paso 2).

```bash
ls -lh ~
```

```
-rw-r--r--  549  archivo_x.txt.zip
```

Ese `.zip` es el paquete cifrado. Todavía no se puede leer su contenido.

---

## Paso 7 · Restaurar en la sede

> Dentro de la **sede**, en bash.

Se indica qué paquetes recuperar y dónde dejar los originales:

```bash
cd ~/secure-file-transfer-node
./bin/sftnode receive --file ~/archivo_x.txt.zip --dest ~
```

Varios a la vez:

```bash
./bin/sftnode receive \
  --file ~/Desarrollo-23-02-2026.CiudadMexico.zip \
  --file ~/Comercial-10-11-2026.Santiago.zip \
  --dest ~
```

El programa verifica el hash, descifra, decodifica y escribe el archivo
original:

```
· 1 paquete(s) por restaurar en /home/sede
✔ archivo_x.txt  verificado, descifrado y restaurado
Resumen: 1 restaurados · 0 alertas · 0 errores
```

Comprobar el contenido recuperado:

```bash
cat ~/archivo_x.txt
```

Si prefiere llamar al programa desde cualquier carpeta:

```bash
echo 'export PATH=$PATH:$HOME/secure-file-transfer-node/bin' >> ~/.bashrc
source ~/.bashrc
sftnode help
```

---

## Resumen del recorrido

| Paso | Dónde | Qué se hace |
|---|---|---|
| 1 | Windows | levantar el contenedor de la sede |
| 2 | Windows | entrar a la sede por SSH |
| 3 | Sede | variable de entorno, clonar el repositorio y compilar |
| 4 | Windows | compilar el cliente y definir las variables |
| 5 | Windows | enviar los archivos |
| 6 | Sede | comprobar que llegó el paquete |
| 7 | Sede | restaurar los archivos originales |

---

## Opciones

### `send`

| Opción | Obligatoria | Efecto |
|---|---|---|
| `--url` | sí | servidor `sftp://usuario@host:puerto` |
| `--file` | sí | archivo a enviar; se puede repetir o usar comodines |
| `--dest` | no | carpeta remota donde guardar los paquetes |
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
      | ZIP                                                      |
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
| `deployments/` | imagen Docker de una sede receptora, con Go y Git |

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
| `go: command not found` en la sede | use `bash -lc "..."`, que carga el PATH de Go |
| `La carpeta ... no existe en el servidor` | créela antes, o deje que use la de respaldo |
| `no se pudo conectar` | el contenedor está detenido (`docker start sede_mexico`) |
| rutas convertidas a `C:/Program Files/Git/...` | se ejecutó en Git Bash; use PowerShell |
| los archivos de la sede desaparecieron | el contenedor se recreó; `docker run` sin volúmenes no conserva datos |
