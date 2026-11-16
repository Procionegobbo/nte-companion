package ratelimit

import "testing"

func TestAllowPerMinute(t *testing.T) {
	l := New(10, 1000)
	defer l.Close()

	for i := 0; i < 10; i++ {
		ok, _ := l.Allow("1.2.3.4")
		if !ok {
			t.Fatalf("richiesta %d inattesamente rifiutata", i+1)
		}
	}
	ok, retryAfter := l.Allow("1.2.3.4")
	if ok {
		t.Fatalf("l'11a richiesta nello stesso minuto doveva essere rifiutata")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter dovrebbe essere positivo, got %v", retryAfter)
	}
}

func TestAllowIndependentIPs(t *testing.T) {
	l := New(1, 1000)
	defer l.Close()

	if ok, _ := l.Allow("1.1.1.1"); !ok {
		t.Fatal("prima richiesta da 1.1.1.1 doveva passare")
	}
	if ok, _ := l.Allow("1.1.1.1"); ok {
		t.Fatal("seconda richiesta da 1.1.1.1 doveva essere rifiutata")
	}
	if ok, _ := l.Allow("2.2.2.2"); !ok {
		t.Fatal("1.1.1.1 non deve influenzare il limite di 2.2.2.2")
	}
}

func TestAllowDailyCap(t *testing.T) {
	l := New(1000, 3)
	defer l.Close()

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("9.9.9.9"); !ok {
			t.Fatalf("richiesta %d doveva passare (dentro il tetto giornaliero)", i+1)
		}
	}
	if ok, _ := l.Allow("9.9.9.9"); ok {
		t.Fatal("la 4a richiesta doveva superare il tetto giornaliero")
	}
}
