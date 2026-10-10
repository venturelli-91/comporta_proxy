package lgpd

import (
	"strings"
	"testing"
)

func TestMascarar(t *testing.T) {
	casos := []struct {
		nome, entrada, quer string
	}{
		{"CPF com pontuação", "meu CPF é 529.982.247-25", "meu CPF é [CPF_1]"},
		{"CPF sem pontuação", "cpf 52998224725.", "cpf [CPF_1]."},
		{"CPF com dígito inválido fica", "pedido 529.982.247-26", "pedido 529.982.247-26"},
		{"CPF com dígitos repetidos fica", "111.111.111-11", "111.111.111-11"},
		{"CNPJ com pontuação", "empresa 11.222.333/0001-81", "empresa [CNPJ_1]"},
		{"CNPJ sem pontuação", "cnpj 11222333000181", "cnpj [CNPJ_1]"},
		{"CNPJ inválido fica", "cnpj 11.222.333/0001-80", "cnpj 11.222.333/0001-80"},
		{"e-mail", "escreva para ana.souza+loja@exemplo.com.br hoje", "escreva para [EMAIL_1] hoje"},
		{"celular com DDD", "liga (11) 98765-4321", "liga [TELEFONE_1]"},
		{"celular com +55", "whats +55 11 98765-4321", "whats [TELEFONE_1]"},
		{"celular sem formatação", "11987654321", "[TELEFONE_1]"},
		{"fixo com DDD", "fixo (21) 3456-7890", "fixo [TELEFONE_1]"},
		{"cartão válido (Luhn)", "cartão 4111 1111 1111 1111", "cartão [CARTAO_1]"},
		{"cartão inválido fica", "cartão 4111 1111 1111 1112", "cartão 4111 1111 1111 1112"},
		{"número curto fica", "pedido 12345 de 2026", "pedido 12345 de 2026"},
		{"CPF dentro de número maior fica", "protocolo 9529982247251", "protocolo 9529982247251"},
		{"mesmo valor, mesmo marcador", "529.982.247-25 e de novo 529.982.247-25", "[CPF_1] e de novo [CPF_1]"},
		{"valores diferentes, marcadores diferentes", "a@b.com e c@d.com", "[EMAIL_1] e [EMAIL_2]"},
		{"vários tipos", "CPF 529.982.247-25, e-mail a@b.com", "CPF [CPF_1], e-mail [EMAIL_1]"},
		{"marcador literal do usuário não é reaproveitado", "[CPF_1] é meu apelido, CPF 529.982.247-25", "[CPF_1] é meu apelido, CPF [CPF_2]"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			m := NovoMapa()
			got := m.Mascarar(c.entrada)
			if got != c.quer {
				t.Fatalf("Mascarar(%q)\n got %q\nquer %q", c.entrada, got, c.quer)
			}
			if volta := m.Restaurar(got); volta != c.entrada {
				t.Fatalf("Restaurar devolveu %q, queria %q", volta, c.entrada)
			}
		})
	}
}

func TestLenContaValoresDistintos(t *testing.T) {
	m := NovoMapa()
	m.Mascarar("529.982.247-25 529.982.247-25 a@b.com")
	if m.Len() != 2 {
		t.Fatalf("Len = %d, queria 2", m.Len())
	}
}

func TestRestaurarIgnoraMarcadorDesconhecido(t *testing.T) {
	m := NovoMapa()
	m.Mascarar("a@b.com")
	if got := m.Restaurar("[EMAIL_1] e [CPF_9]"); got != "a@b.com e [CPF_9]" {
		t.Fatalf("got %q", got)
	}
}

func TestStreamComMarcadorPartido(t *testing.T) {
	m := NovoMapa()
	m.Mascarar("meu CPF é 529.982.247-25")
	s := m.NovoStream()
	var b strings.Builder
	for _, p := range []string{"Seu CPF ", "[CP", "F_1", "] está ok [", "x"} {
		b.WriteString(s.Escrever(p))
	}
	b.WriteString(s.Finalizar())
	if got := b.String(); got != "Seu CPF 529.982.247-25 está ok [x" {
		t.Fatalf("got %q", got)
	}
}

func FuzzMascararRestaurar(f *testing.F) {
	for _, s := range []string{
		"meu CPF é 529.982.247-25", "[CPF_1] 52998224725", "a@b.com [EMAIL_1] [EMAIL_",
		"(11) 98765-4321 4111111111111111 11.222.333/0001-81", "[[CPF_1]]", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m := NovoMapa()
		if volta := m.Restaurar(m.Mascarar(s)); volta != s {
			t.Fatalf("Restaurar(Mascarar(%q)) = %q", s, volta)
		}
	})
}

// FuzzStream: qualquer forma de cortar a resposta em pedaços dá o mesmo resultado que
// restaurar o texto inteiro de uma vez.
func FuzzStream(f *testing.F) {
	f.Add("a@b.com 529.982.247-25", "x [EMAIL_1] y [CPF_1] z [EMA", uint8(3))
	f.Fuzz(func(t *testing.T, entrada, resposta string, passo uint8) {
		m := NovoMapa()
		m.Mascarar(entrada)
		quer := m.Restaurar(resposta)
		n := int(passo%7) + 1
		s := m.NovoStream()
		var b strings.Builder
		for i := 0; i < len(resposta); i += n {
			b.WriteString(s.Escrever(resposta[i:min(i+n, len(resposta))]))
		}
		b.WriteString(s.Finalizar())
		if b.String() != quer {
			t.Fatalf("stream = %q, inteiro = %q", b.String(), quer)
		}
	})
}
