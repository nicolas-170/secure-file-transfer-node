// Package cli interpreta los argumentos y ejecuta el comando pedido.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolas/sftnode/internal/config"
	"github.com/nicolas/sftnode/internal/envio"
	"github.com/nicolas/sftnode/internal/logx"
	"github.com/nicolas/sftnode/internal/naming"
	"github.com/nicolas/sftnode/internal/paquete"
)

// Códigos de salida, para que cron o systemd sepan qué pasó sin leer el log.
const (
	salidaOK       = 0
	salidaConfig   = 2
	salidaConexion = 3
)

// Run ejecuta el subcomando y devuelve el código de salida.
func Run(args []string) int {
	if len(args) == 0 {
		ayuda()
		return salidaConfig
	}

	switch args[0] {
	case "send":
		return enviar(args[1:])
	case "receive":
		return recibir(args[1:])
	case "help", "--help", "-h":
		ayuda()
		return salidaOK
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", args[0])
		ayuda()
		return salidaConfig
	}
}

// enviar aplica a cada archivo las seis etapas del procedimiento.
func enviar(args []string) int {
	c, err := config.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return salidaConfig
	}
	if c.SinColor {
		logx.SinColor()
	}

	logx.Info("Servidor %s  usuario=%s", c.Servidor(), c.Usuario)
	logx.Info("%d archivo(s) por enviar", len(c.Archivos))

	// Los nombres se revisan antes de conectarse, para fallar cuanto antes.
	archivos := revisarNombres(c)
	if len(archivos) == 0 {
		logx.Error("No hay archivos que enviar")
		return logx.Resumen(0, "enviados")
	}

	if c.DryRun {
		logx.OK("Simulación: no se cifró ni se transmitió nada")
		return logx.Resumen(0, "enviados")
	}

	cliente, err := envio.Conectar(c)
	if err != nil {
		logx.Error("%v", err)
		return salidaConexion
	}
	defer cliente.Cerrar()
	logx.OK("Canal seguro SSH establecido")

	// Carpeta destino: la pedida si ya existe, si no la del sistema.
	carpeta, err := cliente.ElegirCarpeta(c.Destino, c.CarpetasPorDefecto())
	if err != nil {
		logx.Error("%v", err)
		return salidaConexion
	}
	if c.Destino != "" && carpeta != c.Destino {
		logx.Alerta("La carpeta %s no existe en el servidor; se guardará en %s", c.Destino, carpeta)
	}
	logx.Info("Carpeta destino %s", carpeta)

	enviados := 0
	for _, a := range archivos {
		if procesar(cliente, c, a, carpeta) {
			enviados++
		}
	}

	return logx.Resumen(enviados, "enviados")
}

// procesar aplica el pipeline a un archivo. Devuelve true si llegó bien.
func procesar(cliente *envio.Cliente, c config.Config, a naming.Archivo, carpeta string) bool {
	logx.Paso("%s  (1-4) Base64 + AES-256-GCM + SHA-256 + ZIP", a.Nombre)

	datos, m, err := paquete.Crear(a.Ruta, paquete.Manifiesto{
		Nombre: a.Nombre, Area: a.Area, Fecha: a.Fecha, Sede: a.Sede,
	}, c.Passphrase)
	if err != nil {
		logx.Error("%s  no se pudo empaquetar: %v", a.Nombre, err)
		return false
	}
	logx.Info("%s  hash=%s...  tamaño=%d bytes", a.Nombre, m.Hash[:12], len(datos))

	zip := a.Nombre + ".zip"
	logx.Paso("%s  (5) transmitiendo por SFTP", a.Nombre)

	if err := cliente.Enviar(datos, zip, carpeta); err != nil {
		logx.Error("%s  fallo al transmitir: %v", a.Nombre, err)
		return false
	}
	if err := cliente.Verificar(zip, carpeta, paquete.Hash(datos)); err != nil {
		logx.Error("%s  (6) integridad: %v", a.Nombre, err)
		return false
	}

	logx.OK("%s  guardado y verificado en %s", zip, carpeta)
	return true
}

// revisarNombres comprueba la nomenclatura Area-DD-MM-AAAA.Sede. Los nombres
// que no la cumplen se avisan en amarillo y se envían igual, salvo con
// --strict-naming.
func revisarNombres(c config.Config) []naming.Archivo {
	var archivos []naming.Archivo

	for _, ruta := range c.Archivos {
		a, err := naming.Parse(ruta)
		if err != nil {
			if c.Estricto {
				logx.Error("%v", err)
				continue
			}
			logx.Alerta("%v (se envía igualmente)", err)
			a = naming.Archivo{Ruta: ruta, Nombre: filepath.Base(ruta)}
		}
		archivos = append(archivos, a)
	}
	return archivos
}

// recibir restaura los paquetes que se le indiquen. Igual que en el envío,
// se procesan únicamente los archivos señalados con --file.
func recibir(args []string) int {
	var patrones config.Lista
	fs := flag.NewFlagSet("receive", flag.ContinueOnError)
	fs.Var(&patrones, "file", "paquete .zip a restaurar (se puede repetir)")
	destino := fs.String("dest", "", "carpeta donde dejar los originales; si no se puede usar, la del sistema")
	sinColor := fs.Bool("no-color", false, "salida sin color")
	if err := fs.Parse(args); err != nil {
		return salidaConfig
	}
	if *sinColor {
		logx.SinColor()
	}

	passphrase := os.Getenv("SFT_PASSPHRASE")
	if passphrase == "" {
		fmt.Fprintln(os.Stderr, "error: falta la variable SFT_PASSPHRASE")
		return salidaConfig
	}
	if len(patrones) == 0 {
		fmt.Fprintln(os.Stderr, "error: falta --file con el paquete o los paquetes a restaurar")
		return salidaConfig
	}

	paquetes, err := config.Expandir(patrones)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return salidaConfig
	}

	carpeta := carpetaSalida(*destino)
	logx.Info("%d paquete(s) por restaurar en %s", len(paquetes), carpeta)

	restaurados := 0
	for _, ruta := range paquetes {
		nombre, original, err := abrirPaquete(ruta, passphrase)
		if err != nil {
			logx.Error("%s  %v", filepath.Base(ruta), err)
			continue
		}
		if err := os.WriteFile(filepath.Join(carpeta, nombre), original, 0o640); err != nil {
			logx.Error("%s  %v", nombre, err)
			continue
		}
		logx.OK("%s  verificado, descifrado y restaurado", nombre)
		restaurados++
	}

	return logx.Resumen(restaurados, "restaurados")
}

func abrirPaquete(ruta, passphrase string) (string, []byte, error) {
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return "", nil, err
	}
	return paquete.Abrir(datos, passphrase)
}

// carpetaSalida usa la carpeta pedida, que debe existir ya, y si no recurre a
// la carpeta temporal del sistema, disponible en Linux y en Windows.
func carpetaSalida(pedida string) string {
	if pedida != "" {
		if info, err := os.Stat(pedida); err == nil && info.IsDir() {
			return pedida
		}
		logx.Alerta("La carpeta %s no existe", pedida)
	}

	respaldo := filepath.Join(os.TempDir(), "sftnode-restaurados")
	os.MkdirAll(respaldo, 0o750)
	return respaldo
}

func ayuda() {
	fmt.Fprint(os.Stderr, `sftnode - transferencia segura de archivos entre sedes

  sftnode send    --url sftp://usuario@host:puerto --file <archivo> --dest <carpeta>
  sftnode receive --file <paquete.zip> --dest <carpeta>

Ejemplos:
  $env:SFT_PASSPHRASE   = "ClaveDeCifrado"
  $env:SFT_SSH_PASSWORD = "sede123"

  sftnode send --url sftp://sede@localhost:2201 ^
               --file Desarrollo-23-02-2026.CiudadMexico ^
               --dest /home/sede/incoming

  sftnode receive --file /home/sede/incoming/Desarrollo-23-02-2026.CiudadMexico.zip ^
                  --dest /home/sede/restaurados

Opciones de send:
  --file               archivo a enviar; se puede repetir o usar comodines
  --dest               carpeta remota donde guardar los paquetes
  --key-file           clave privada SSH          (~/.ssh/id_ed25519)
  --known-hosts        host keys conocidas        (~/.ssh/known_hosts)
  --dry-run            valida sin cifrar ni enviar
  --strict-naming      aborta si un nombre no cumple la nomenclatura
  --insecure-host-key  omite la verificación del servidor (solo laboratorio)
  --no-color           salida sin color

Opciones de receive:
  --file               paquete .zip a restaurar; se puede repetir
  --dest               carpeta donde dejar los archivos originales

Si la carpeta indicada no existe ni se puede crear, se usan las del sistema:
  Linux    /home/<usuario>/sftnode  o  /tmp/sftnode
  Windows  C:/Users/<usuario>/sftnode

Variables de entorno:
  SFT_PASSPHRASE       clave del cifrado AES-256-GCM (obligatoria)
  SFT_SSH_PASSWORD     contraseña SSH, si no se usa clave privada
`)
}
