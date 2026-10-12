package lgpd

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The corpus marks each piece of personal data inline: "meu CPF é {{CPF:529.982.247-25}}".
// Types the detector doesn't cover (NOME, ENDERECO) are labeled too, so recall over all
// personal data is honest.
var rotulo = regexp.MustCompile(`\{\{([A-Z]+):([^}]+)\}\}`)

type dado struct{ tipo, valor string }

type exemplo struct {
	linha int
	texto string
	dados []dado
}

// extrair dedups like Mapa does: the same value twice in a line is one piece of data.
func extrair(linha string) (string, []dado) {
	linha = strings.TrimSpace(linha)
	var dados []dado
	visto := map[dado]bool{}
	for _, m := range rotulo.FindAllStringSubmatch(linha, -1) {
		d := dado{m[1], m[2]}
		if !visto[d] {
			visto[d] = true
			dados = append(dados, d)
		}
	}
	return rotulo.ReplaceAllString(linha, "$2"), dados
}

func lerCorpus(t *testing.T, caminho string) []exemplo {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	var out []exemplo
	for i, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		texto, dados := extrair(l)
		out = append(out, exemplo{linha: i + 1, texto: texto, dados: dados})
	}
	return out
}

// detectados runs the real masking and reads back what it replaced. The type is the
// placeholder's prefix: "[CPF_2]" → "CPF".
func detectados(texto string) []dado {
	m := NovoMapa()
	m.Mascarar(texto)
	var out []dado
	for mk, v := range m.valores {
		out = append(out, dado{mk[1:strings.LastIndexByte(mk, '_')], v})
	}
	return out
}

func TestExtrair(t *testing.T) {
	casos := []struct {
		nome, linha, texto string
		dados              []dado
	}{
		{"no tags", "pedido 123", "pedido 123", nil},
		{"one tag", "CPF {{CPF:529.982.247-25}} ok", "CPF 529.982.247-25 ok", []dado{{"CPF", "529.982.247-25"}}},
		{"two types", "{{EMAIL:a@exemplo.com}} e {{NOME:Ana Lima}}", "a@exemplo.com e Ana Lima",
			[]dado{{"EMAIL", "a@exemplo.com"}, {"NOME", "Ana Lima"}}},
		{"duplicate value counted once", "{{CPF:529.982.247-25}} {{CPF:529.982.247-25}}",
			"529.982.247-25 529.982.247-25", []dado{{"CPF", "529.982.247-25"}}},
		{"crlf", "fone {{TELEFONE:(11) 98765-4321}}\r", "fone (11) 98765-4321", []dado{{"TELEFONE", "(11) 98765-4321"}}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			texto, dados := extrair(c.linha)
			if texto != c.texto || !reflect.DeepEqual(dados, c.dados) {
				t.Fatalf("extrair(%q) = %q %v, want %q %v", c.linha, texto, dados, c.texto, c.dados)
			}
		})
	}
}

func TestDetectados(t *testing.T) {
	got := detectados("CPF 529.982.247-25, a@exemplo.com")
	want := map[dado]bool{{"CPF", "529.982.247-25"}: true, {"EMAIL", "a@exemplo.com"}: true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, d := range got {
		if !want[d] {
			t.Fatalf("unexpected %v in %v", d, got)
		}
	}
}
