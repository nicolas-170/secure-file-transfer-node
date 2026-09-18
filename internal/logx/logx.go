// Package logx: mensajes de consola con color.
// Verde = bien, amarillo = alerta, rojo = error, gris = informativo.
package logx

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	reset  = "\033[0m"
	gray   = "\033[90m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
)

var (
	usarColor = true
	inicio    = time.Now()
	alertas   int
	errores   int
)

// SinColor desactiva los códigos ANSI (flag --no-color o salida a un archivo).
func SinColor() { usarColor = false }

// escribir imprime una línea: hora + símbolo + mensaje, todo del mismo color.
func escribir(color, simbolo, formato string, args ...any) {
	texto := fmt.Sprintf("%s %s %s", time.Now().Format("15:04:05"), simbolo,
		fmt.Sprintf(formato, args...))
	if usarColor {
		texto = color + texto + reset
	}
	fmt.Println(texto)
}

// Info: avance normal del proceso.
func Info(f string, a ...any) { escribir(gray, "·", f, a...) }

// Paso: inicio de una etapa del pipeline.
func Paso(f string, a ...any) { escribir(cyan, "▶", f, a...) }

// OK: operación completada con éxito.
func OK(f string, a ...any) { escribir(green, "✔", f, a...) }

// Alerta: algo recuperable que conviene mirar.
func Alerta(f string, a ...any) { alertas++; escribir(yellow, "⚠", f, a...) }

// Error: fallo que aborta el archivo en curso.
func Error(f string, a ...any) { errores++; escribir(red, "✖", f, a...) }

// Resumen imprime el balance final y devuelve el código de salida:
// 0 si no hubo errores, 1 si hubo al menos uno.
func Resumen(cantidad int, etiqueta string) int {
	linea := fmt.Sprintf("Resumen: %d %s · %d alertas · %d errores · %s",
		cantidad, etiqueta, alertas, errores, time.Since(inicio).Round(time.Millisecond))

	color := green
	if alertas > 0 {
		color = yellow
	}
	if errores > 0 {
		color = red
	}

	fmt.Println(strings.Repeat("-", 60))
	if usarColor {
		linea = color + linea + reset
	}
	fmt.Println(linea)

	if errores > 0 {
		return 1
	}
	return 0
}

// init desactiva el color si la salida no es una terminal o si existe NO_COLOR.
func init() {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || os.Getenv("NO_COLOR") != "" {
		usarColor = false
	}
}
