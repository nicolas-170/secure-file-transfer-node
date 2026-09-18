// Package naming valida la nomenclatura corporativa: Area-DD-MM-AAAA.Sede
// Ejemplo: Desarrollo-23-02-2026.CiudadMexico
package naming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var patron = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)-(\d{2})-(\d{2})-(\d{4})\.([A-Za-z]+)$`)

// Archivo son las partes del nombre ya separadas.
type Archivo struct {
	Ruta   string
	Nombre string
	Area   string
	Fecha  string
	Sede   string
}

// Parse separa el nombre en área, fecha y sede. Falla si no cumple el formato
// o si la fecha no existe en el calendario (por ejemplo 31-02-2026).
func Parse(ruta string) (Archivo, error) {
	nombre := filepath.Base(ruta)

	p := patron.FindStringSubmatch(nombre)
	if p == nil {
		return Archivo{}, fmt.Errorf("%q no cumple el formato Area-DD-MM-AAAA.Sede", nombre)
	}

	dia, _ := strconv.Atoi(p[2])
	mes, _ := strconv.Atoi(p[3])
	anio, _ := strconv.Atoi(p[4])

	// time.Date convierte fechas imposibles en la siguiente válida,
	// así que se compara el resultado con lo escrito para detectarlas.
	f := time.Date(anio, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
	if f.Day() != dia || int(f.Month()) != mes || f.Year() != anio {
		return Archivo{}, fmt.Errorf("%q tiene una fecha inexistente", nombre)
	}

	return Archivo{
		Ruta:   ruta,
		Nombre: nombre,
		Area:   p[1],
		Fecha:  f.Format("02-01-2006"),
		Sede:   p[5],
	}, nil
}
