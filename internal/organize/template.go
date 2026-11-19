// Package organize renomeia e move as edições para o padrão de pastas e nomes configurado (RF-40):
// gera um plano com prévia, aplica só o aprovado e guarda o histórico para desfazer (RF-41, RF-42).
package organize

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Variables são as variáveis aceitas no template, com o que cada uma vira.
var Variables = map[string]string{
	"editora":  "editora (da fonte de metadados; sem ela, a primeira pasta da biblioteca)",
	"serie":    "série (da fonte; sem ela, a identificada pelo nome)",
	"ano":      "ano",
	"numero":   "número da edição (1, 12)",
	"numero3":  "número com 3 dígitos (001, 012)",
	"volume":   "volume (o 3 de \"v3\")",
	"titulo":   "título da edição (da fonte)",
	"original": "nome atual do arquivo, sem extensão",
}

// required são as variáveis sem as quais a edição não é organizada (o nome ficaria ambíguo).
var required = map[string]string{"serie": "série", "numero": "número", "numero3": "número"}

var varPattern = regexp.MustCompile(`\{([a-z0-9]+)\}`)

// ErrTemplate indica template inválido.
var ErrTemplate = errors.New("template inválido")

// CheckTemplate confere as variáveis do template.
func CheckTemplate(tpl string) error {
	for _, m := range varPattern.FindAllStringSubmatch(tpl, -1) {
		if _, ok := Variables[m[1]]; !ok {
			return fmt.Errorf("%w: variável desconhecida {%s}", ErrTemplate, m[1])
		}
	}
	if !strings.Contains(tpl, "{serie}") {
		return fmt.Errorf("%w: precisa ter {serie}", ErrTemplate)
	}
	if strings.HasPrefix(tpl, "/") || strings.Contains(tpl, "..") || strings.Contains(tpl, "\\") {
		return fmt.Errorf("%w: use só caminhos relativos à biblioteca, com /, sem ..", ErrTemplate)
	}
	return nil
}

// Render aplica o template e devolve o caminho relativo (pasta/nome, sem extensão) e o que faltou
// entre as variáveis obrigatórias usadas. Partes vazias somem com o que as envolve: "Série () #" vira
// "Série". Cada pasta/nome é limpo dos caracteres que exFAT/NTFS recusam.
func Render(tpl string, v map[string]string) (string, []string) {
	var missing []string
	seen := map[string]bool{}
	out := varPattern.ReplaceAllStringFunc(tpl, func(m string) string {
		name := m[1 : len(m)-1]
		val := strings.TrimSpace(v[name])
		if label, ok := required[name]; ok && val == "" && !seen[label] {
			seen[label] = true
			missing = append(missing, label)
		}
		return strings.ReplaceAll(val, "/", "-") // valor nunca cria pasta
	})
	var parts []string
	for _, seg := range strings.Split(out, "/") {
		if seg = cleanSegment(seg); seg != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "/"), missing
}

var (
	emptyGroups = regexp.MustCompile(`\(\s*\)|\[\s*\]|\{\s*\}`)
	spaces      = regexp.MustCompile(`\s+`)
	forbidden   = strings.NewReplacer(":", " -", "\\", "-", "*", "", "?", "", "\"", "'", "<", "", ">", "", "|", "-")
)

// maxSegment limita cada pasta/nome (bytes), abaixo do limite de 255 dos sistemas de arquivos.
const maxSegment = 150

func cleanSegment(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = forbidden.Replace(s)
	s = emptyGroups.ReplaceAllString(s, "")
	s = spaces.ReplaceAllString(s, " ")
	s = strings.Trim(s, " .-_#")
	for len(s) > maxSegment {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return strings.TrimRight(s, " .")
}
