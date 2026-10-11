package lgpd

import (
	"strings"
	"testing"
)

func TestMascarar(t *testing.T) {
	casos := []struct {
		nome, entrada, quer string
	}{
		{"CPF with punctuation", "meu CPF é 529.982.247-25", "meu CPF é [CPF_1]"},
		{"CPF without punctuation", "cpf 52998224725.", "cpf [CPF_1]."},
		{"CPF with invalid digit stays", "pedido 529.982.247-26", "pedido 529.982.247-26"},
		{"CPF with repeated digits stays", "111.111.111-11", "111.111.111-11"},
		{"CNPJ with punctuation", "empresa 11.222.333/0001-81", "empresa [CNPJ_1]"},
		{"CNPJ without punctuation", "cnpj 11222333000181", "cnpj [CNPJ_1]"},
		{"invalid CNPJ stays", "cnpj 11.222.333/0001-80", "cnpj 11.222.333/0001-80"},
		{"email", "escreva para ana.souza+loja@exemplo.com.br hoje", "escreva para [EMAIL_1] hoje"},
		{"mobile with area code", "liga (11) 98765-4321", "liga [TELEFONE_1]"},
		{"mobile with +55", "whats +55 11 98765-4321", "whats [TELEFONE_1]"},
		{"unformatted mobile", "11987654321", "[TELEFONE_1]"},
		{"landline with area code", "fixo (21) 3456-7890", "fixo [TELEFONE_1]"},
		{"valid card (Luhn)", "cartão 4111 1111 1111 1111", "cartão [CARTAO_1]"},
		{"invalid card stays", "cartão 4111 1111 1111 1112", "cartão 4111 1111 1111 1112"},
		{"short number stays", "pedido 12345 de 2026", "pedido 12345 de 2026"},
		{"CPF inside a bigger number stays", "protocolo 9529982247251", "protocolo 9529982247251"},
		{"same value, same placeholder", "529.982.247-25 e de novo 529.982.247-25", "[CPF_1] e de novo [CPF_1]"},
		{"different values, different placeholders", "a@b.com e c@d.com", "[EMAIL_1] e [EMAIL_2]"},
		{"several types", "CPF 529.982.247-25, e-mail a@b.com", "CPF [CPF_1], e-mail [EMAIL_1]"},
		{"user's literal placeholder is not reused", "[CPF_1] é meu apelido, CPF 529.982.247-25", "[CPF_1] é meu apelido, CPF [CPF_2]"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			m := NovoMapa()
			got := m.Mascarar(c.entrada)
			if got != c.quer {
				t.Fatalf("Mascarar(%q)\n got %q\nwant %q", c.entrada, got, c.quer)
			}
			if volta := m.Restaurar(got); volta != c.entrada {
				t.Fatalf("Restaurar returned %q, want %q", volta, c.entrada)
			}
		})
	}
}

func TestLenContaValoresDistintos(t *testing.T) {
	m := NovoMapa()
	m.Mascarar("529.982.247-25 529.982.247-25 a@b.com")
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
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

// FuzzStream: any way of cutting the response into chunks gives the same result as
// restoring the whole text at once.
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
			t.Fatalf("stream = %q, whole = %q", b.String(), quer)
		}
	})
}

// A literal placeholder in one message can't be reused for another message's real data.
func TestMascararVariosNaoColideEntreMensagens(t *testing.T) {
	m := NovoMapa()
	in := []string{"use o rótulo [CPF_1] no formulário", "meu CPF é 529.982.247-25"}
	out := m.MascararVarios(in)
	if out[1] != "meu CPF é [CPF_2]" {
		t.Fatalf("second message = %q, want [CPF_2]", out[1])
	}
	if got := m.Restaurar("O campo [CPF_1] foi preenchido"); got != "O campo [CPF_1] foi preenchido" {
		t.Fatalf("restoring replaced the user's literal: %q", got)
	}
	for i := range in {
		if m.Restaurar(out[i]) != in[i] {
			t.Fatalf("round trip of message %d: %q", i, m.Restaurar(out[i]))
		}
	}
}

func FuzzMascararVarios(f *testing.F) {
	f.Add("[CPF_1]", "529.982.247-25")
	f.Fuzz(func(t *testing.T, a, b string) {
		m := NovoMapa()
		out := m.MascararVarios([]string{a, b})
		if m.Restaurar(out[0]) != a || m.Restaurar(out[1]) != b {
			t.Fatalf("round trip failed: %q %q", a, b)
		}
	})
}
