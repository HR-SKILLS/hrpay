# HR-Skills Pay Go SDK

> SDK Go officiel pour [HR-Skills Pay](https://hrskills-pay.com) — Mobile Money, Portefeuilles, Airtime, Factures, Payroll, Cartes Virtuelles et plus.

Infrastructure de paiement B2B pour l'Afrique — **16 pays** : Cameroun, Côte d'Ivoire, Sénégal, Gabon, RD Congo, Mali, Burkina Faso, Togo, Bénin, Guinée, Gambie et plus.

|                    |                                                                |
| ------------------ | -------------------------------------------------------------- |
| **Base URL** | `https://api.hrskills-pay.com`                               |
| **Frais**    | 1,5 % Cash-In · 1 % Cash-Out                                  |
| **Version**  | v1 · REST JSON                                                |
| **Sandbox**  | Montant**pair → SUCCESS** · **impair → FAILED** |

---

## Table des matières

1. [Installation](#installation)
2. [Sandbox vs Production](#sandbox-vs-production)
3. [Démarrage rapide](#démarrage-rapide)
4. [Authentification](#authentification)
5. [Cash-In — Collecte Mobile Money](#cash-in--collecte-mobile-money)
6. [Cash-Out — Envoi de fonds](#cash-out--envoi-de-fonds)
7. [Statuts &amp; Transactions](#statuts--transactions)
8. [Solde &amp; Mouvements](#solde--mouvements)
9. [Services VAS — Airtime, Data, Factures](#services-vas--airtime-data-factures)
10. [Commissions revendeur](#commissions-revendeur)
11. [Payroll — Paiements de masse](#payroll--paiements-de-masse)
12. [Cartes virtuelles](#cartes-virtuelles)
13. [Liens de paiement](#liens-de-paiement)
14. [Webhooks](#webhooks)
15. [Opérateurs &amp; Pays](#opérateurs--pays)
16. [Gestion des erreurs](#gestion-des-erreurs)
17. [Configuration &amp; Résilience](#configuration--résilience)

---

## Installation

```bash
go get github.com/hr-skills/hrpay
```

Go 1.22+ requis. Seule dépendance externe : `google/uuid` (idempotence).

---

## Sandbox vs Production

> ⚠️ **Point le plus important à comprendre.** Sandbox et production partagent **exactement la même URL** (`https://api.hrskills-pay.com`). C'est **la clé API** qui détermine l'environnement, pas l'hôte. Il n'y a **pas** de chemin `/sandbox`.

|                      | SANDBOX · TEST                             | PRODUCTION · LIVE                                       |
| -------------------- | ------------------------------------------- | -------------------------------------------------------- |
| **Clés**      | `hrsk_pk_test_...` / `hrsk_sk_test_...` | `hrsk_pk_live_...` / `hrsk_sk_live_...`              |
| **URL**        | `https://api.hrskills-pay.com`            | `https://api.hrskills-pay.com` (identique)             |
| **Paiements**  | Aucun paiement réel                        | Paiements réels MTN / Orange                            |
| **Prérequis** | Aucun                                       | **KYC approuvé** (sinon `403 KYC_NOT_APPROVED`) |
| **Résultat**  | Piloté par la**parité du montant**  | Piloté par le client réel                              |

### 🎲 La règle de parité en sandbox

En sandbox, le statut final est déterminé par **la parité du montant** :

| Montant                             | Statut final   | Usage                      |
| ----------------------------------- | -------------- | -------------------------- |
| **Pair** (5000, 1000, 200…)  | `SUCCESS` ✅ | Tester le chemin nominal   |
| **Impair** (5001, 999, 301…) | `FAILED` ❌  | Tester la gestion d'échec |

```go
// SANDBOX — forcer un SUCCESS : montant PAIR
success, _ := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
    PhoneNumber: "237655500393",
    Operator:    hrpay.OperatorOrange,
    Amount:      5000, // pair → SUCCESS
})

// SANDBOX — forcer un FAILED : montant IMPAIR
failed, _ := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
    PhoneNumber: "237655500393",
    Operator:    hrpay.OperatorOrange,
    Amount:      5001, // impair → FAILED
})
```

> ℹ️ Le statut initial reste **toujours** `PENDING` dans les deux cas. La parité détermine le statut **final**, que vous récupérez par polling (`Transactions.Poll`) ou via le webhook. Le montant minimum reste **100** — donc `101` est le plus petit montant qui échoue, `100` le plus petit qui réussit.

### Basculer sandbox → production

Aucun changement de code. Seules les clés changent :

```go
// Sandbox
client, _ := hrpay.NewClient(hrpay.WithAPIKeys(
    os.Getenv("HRPAY_PK_TEST"), os.Getenv("HRPAY_SK_TEST"),
))

// Production — mêmes appels, mêmes URLs
client, _ := hrpay.NewClient(hrpay.WithAPIKeys(
    os.Getenv("HRPAY_PK_LIVE"), os.Getenv("HRPAY_SK_LIVE"),
))

// Vérifier dans quel environnement on tourne (après le 1er appel)
fmt.Println(client.Auth.Environment()) // "TEST" ou "LIVE"
fmt.Println(client.Auth.MerchantID())  // "e6af1e82-..."
```

> 🔒 **Ne committez jamais vos clés.** La Clé B (`hrsk_sk_...`) ne doit **jamais** être exposée côté client (navigateur, app mobile).

---

## Démarrage rapide

Exemple complet et fonctionnel : encaisser 5 000 XAF et attendre le statut final.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hr-skills/hrpay"
)

func main() {
	ctx := context.Background()

	client, err := hrpay.NewClient(
		hrpay.WithAPIKeys(os.Getenv("HRPAY_PK"), os.Getenv("HRPAY_SK")),
		hrpay.WithTimeout(30*time.Second),
		hrpay.WithMaxRetries(3),
	)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	// 1. Vérifier le solde (optionnel pour un Cash-In)
	balance, err := client.Wallet.Balance(ctx)
	if err != nil {
		log.Fatalf("solde: %v", err)
	}
	fmt.Printf("Disponible : %.0f %s (dont %.0f en hold)\n",
		balance.Balance.Available, balance.Currency, balance.Balance.Held)

	// 2. Initier le Cash-In → 202 PENDING
	tx, err := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    hrpay.OperatorOrange,
		Country:     hrpay.CountryCm,
		Amount:      5000, // sandbox : pair → SUCCESS
		Description: "Commande #1024",
		Metadata:    map[string]interface{}{"order_id": "1024"},
	})
	if err != nil {
		log.Fatalf("cash-in: %v", err)
	}

	fmt.Printf("Référence   : %s\n", tx.Reference)   // ref_d5b40df948dc52cc
	fmt.Printf("Statut      : %s\n", tx.Status)      // PENDING
	fmt.Printf("Frais       : %.0f (%.1f%%)\n", tx.Fee, tx.FeePercent) // 75 (1.5%)
	fmt.Printf("Net crédité : %.0f\n", tx.NetAmount) // 4925

	// 3. Le client confirme sur son téléphone (USSD Orange / push MTN).
	//    Sans confirmation → FAILED après 10 min.

	// 4. Polling jusqu'au statut final
	final, err := client.Transactions.Poll(ctx, tx.Reference, hrpay.PollOptions{
		Interval: 10 * time.Second, // cadence recommandée
		Timeout:  10 * time.Minute, // timeout côté API
		OnStatus: func(status string, attempt int) {
			fmt.Printf("  tentative %d → %s\n", attempt, status)
		},
	})
	if err != nil {
		log.Fatalf("polling: %v", err)
	}

	switch final.Status {
	case hrpay.StatusSuccess:
		fmt.Println("✅ Paiement confirmé — net crédité, disponible sous 48h")
	case hrpay.StatusFailed:
		fmt.Println("❌ Paiement échoué ou expiré")
	case hrpay.StatusHold:
		fmt.Println("⏸️  Bloqué par l'AML — révision manuelle en cours")
	}

	_ = errors.Is // cf. section Gestion des erreurs
}
```

---

## Authentification

Le SDK gère **automatiquement** le mécanisme à double clé + transaction token. Vous n'avez normalement **rien à faire**.

|                                    | Rôle                                                                   |
| ---------------------------------- | ----------------------------------------------------------------------- |
| **Clé A** (`hrsk_pk_...`) | Envoyée dans`Authorization: Bearer` à chaque requête               |
| **Clé B** (`hrsk_sk_...`) | Échangée**une seule fois** contre un JWT. Jamais côté client. |
| **Transaction Token**        | JWT HMAC-SHA256, TTL**45 min** (`expires_in: 2700`)             |

Le SDK :

- échange la Clé B contre un token au premier appel,
- le met en cache (mémoire ou fichier),
- le **rafraîchit automatiquement** 60 s avant expiration,
- protège le rafraîchissement par mutex (pas de *refresh storm* en concurrence).

### API bas niveau (rarement nécessaire)

```go
token, err := client.Auth.GetToken(ctx)      // token actif (depuis le cache)
token, err = client.Auth.Refresh(ctx)        // forcer le rafraîchissement
valid := client.Auth.IsTokenValid(ctx)       // validité locale
expiry, err := client.Auth.GetTokenExpiry(ctx)
err = client.Auth.ClearCache(ctx)

// Métadonnées renvoyées par /v1/auth/transaction-token
merchantID := client.Auth.MerchantID()  // "e6af1e82-..."
env := client.Auth.Environment()        // "LIVE" ou "TEST"
```

### Persistance du token entre redémarrages

```go
client, _ := hrpay.NewClient(
	hrpay.WithAPIKeys(pk, sk),
	hrpay.WithTokenCache(hrpay.NewFileTokenCache("/var/lib/myapp/hrpay-token.json")),
)
```

Vous pouvez brancher Redis, Memcached, etc. en implémentant l'interface `TokenCache` :

```go
type TokenCache interface {
	Get(ctx context.Context, key string) (string, time.Time, error)
	Set(ctx context.Context, key, token string, expiresAt time.Time) error
	Delete(ctx context.Context, key string) error
}
```

> L'en-tête `Idempotency-Key` (UUID v4) est injecté automatiquement sur tous les `POST`/`PUT`/`PATCH`.

---

## Cash-In — Collecte Mobile Money

Prélève sur le portefeuille Mobile Money d'un client. **Frais : 1,5 %**.

**Flux :** `POST` → `202 PENDING` → le client confirme (USSD/push) → `SUCCESS` → wallet crédité du `net_amount` → **fonds en hold 48 h** avant d'être disponibles.

### Paramètres

| Champ           | Type     | Requis | Description                                                |
| --------------- | -------- | ------ | ---------------------------------------------------------- |
| `PhoneNumber` | string   | ✅     | Avec indicatif,**sans `+`**. Ex : `237655500393` |
| `Operator`    | Operator | ✅     | `OperatorMtn`, `OperatorOrange`, `OperatorWave`…    |
| `Amount`      | float64  | ✅     | Devise locale.**Minimum 100.** Pas de décimales.    |
| `Country`     | Country  | ✅¹   | ISO 3166-1 alpha-2. Défaut :`CountryCm`                 |
| `Currency`    | Currency | ✅¹   | Défaut :**déduite du pays**                        |
| `Description` | string   | —     | Motif affiché au client sur son téléphone               |
| `Metadata`    | map      | —     | Données libres retournées dans le webhook                |

¹ *Requis par l'API, mais le SDK les remplit pour vous : `Country` → `CM`, `Currency` → devise locale du pays.*

### Exemple — Cameroun (XAF)

```go
tx, err := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
	PhoneNumber: "237655500393",
	Operator:    hrpay.OperatorOrange,
	Country:     hrpay.CountryCm,
	Amount:      5000,
	Description: "Commande #1024",
	Metadata: map[string]interface{}{
		"order_id":    "1024",
		"customer_id": "cust_42",
	},
})
```

Réponse (`202`) — tous les champs sont typés :

```go
tx.TransactionID // "a959b6ca-a5e6-4485-8f92-0747a574b54e"
tx.Reference     // "ref_d5b40df948dc52cc"  ← à stocker pour le suivi
tx.Status        // "PENDING" (toujours, initialement)
tx.Type          // "CASHIN"
tx.Amount        // 5000
tx.Fee           // 75      ← 1,5 % du montant
tx.FeePercent    // 1.5
tx.NetAmount     // 4925    ← crédité après confirmation
tx.Currency      // "XAF"
tx.InitiatedAt   // "2026-06-07T17:00:59Z"
tx.PendingAction // "GET /v1/payments/ref_d5b40df948dc52cc"
```

### Exemple — Sénégal (XOF, Wave)

La devise est déduite automatiquement du pays :

```go
tx, err := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
	PhoneNumber: "221771234567",
	Operator:    hrpay.OperatorWave,
	Country:     hrpay.CountrySn, // → Currency devient XOF
	Amount:      5000,
})
```

### Exemple — RD Congo (CDF, M-Pesa)

```go
tx, err := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
	PhoneNumber: "243810000000",
	Operator:    hrpay.OperatorMpesa,
	Country:     hrpay.CountryCd, // → CDF
	Amount:      10000,
})
```

> ⏱️ **Hold 48 h.** Après `SUCCESS`, le `net_amount` est crédité mais reste dans `balance.held` pendant 48 h. Il n'est **pas dépensable** avant. Voir [Solde &amp; Mouvements](#solde--mouvements).

---

## Cash-Out — Envoi de fonds

Envoie depuis votre wallet vers un compte Mobile Money. **Frais : 1 %**.

> **Prérequis :** `balance.available ≥ montant + 1 %`. Sinon → `402 WALLET_BALANCE_INSUFFICIENT`.

```go
// 1. Toujours vérifier le solde DISPONIBLE avant
balance, err := client.Wallet.Balance(ctx)
if err != nil {
	log.Fatal(err)
}

amount := 5000.0
required := amount * 1.01 // montant + 1 % de frais

if balance.Balance.Available < required {
	log.Fatalf("solde insuffisant : %.0f disponible, %.0f requis",
		balance.Balance.Available, required)
}

// 2. Initier le Cash-Out → 202 PENDING, un hold est posé sur montant + frais
tx, err := client.CashOut.MobileMoney(ctx, hrpay.CashOutMobileMoneyParams{
	PhoneNumber: "237680216505",
	Operator:    hrpay.OperatorMtn,
	Country:     hrpay.CountryCm,
	Amount:      amount,
})
if err != nil {
	// Gérer le solde insuffisant renvoyé par l'API
	if errors.Is(err, hrpay.ErrWalletBalanceInsufficient) {
		var walletErr *hrpay.WalletError
		if errors.As(err, &walletErr) {
			log.Fatalf("il manque %v", walletErr.Details["shortfall"])
		}
	}
	log.Fatal(err)
}

// 3. Polling (10-30 s) ou webhook
final, err := client.Transactions.Poll(ctx, tx.Reference, hrpay.PollOptions{
	Interval: 15 * time.Second,
	Timeout:  10 * time.Minute,
})

// 4. SUCCESS → wallet débité définitivement.
//    ÉCHEC   → hold libéré automatiquement, solde restauré.
```

**Opérateurs destinataires :** `mtn`, `orange`, `moov`, `airtel`, `mpesa`, `wave`.

---

## Statuts & Transactions

| Statut       | Constante                | Signification                                | Terminal ?    |
| ------------ | ------------------------ | -------------------------------------------- | ------------- |
| `PENDING`  | `hrpay.StatusPending`  | En attente de confirmation client / réseau  | non           |
| `SUCCESS`  | `hrpay.StatusSuccess`  | Confirmé — wallet crédité ou débité    | **oui** |
| `FAILED`   | `hrpay.StatusFailed`   | Échec ou timeout (10 min sans confirmation) | **oui** |
| `HOLD`     | `hrpay.StatusHold`     | Bloqué par AML — révision manuelle        | non ⚠️      |
| `REFUNDED` | `hrpay.StatusRefunded` | Remboursé (Cash-In uniquement)              | **oui** |

> ⚠️ `HOLD` **n'est pas terminal** : c'est une révision AML *en cours*. `Poll()` continue donc d'attendre. Si vous voulez sortir immédiatement sur un `HOLD`, utilisez `MaxAttempts` ou testez le statut vous-même via `Status()`.

```go
// Statut d'un paiement — polling toutes les 10 s
tx, err := client.Transactions.Status(ctx, "ref_d5b40df948dc52cc")

// Détail d'une transaction
tx, err = client.Transactions.Get(ctx, "ref_d5b40df948dc52cc")

// Liste paginée — limit max 100 (le SDK rejette au-delà)
page, err := client.Transactions.List(ctx, hrpay.TransactionListParams{
	Status:   hrpay.StatusSuccess,
	Type:     "CASHIN",
	Operator: hrpay.OperatorMtn,
	From:     "2026-06-01",
	To:       "2026-06-30",
	Page:     1,
	Limit:    100,
})
fmt.Printf("%d/%d transactions\n", len(page.Data), page.Meta.Total)
```

### Polling avancé

```go
final, err := client.Transactions.Poll(ctx, ref, hrpay.PollOptions{
	Interval:    10 * time.Second,
	Timeout:     10 * time.Minute,
	MaxAttempts: 60, // borne supplémentaire (optionnel)
	OnStatus: func(status string, attempt int) {
		log.Printf("tentative %d : %s", attempt, status)
	},
})
```

`Poll` s'arrête sur `SUCCESS`, `FAILED` ou `REFUNDED`, et renvoie une erreur `POLL_TIMEOUT` / `POLL_MAX_ATTEMPTS_REACHED` sinon. Il respecte l'annulation du `context`.

> 💡 **Préférez les webhooks au polling** en production. Le polling est un filet de sécurité.

---

## Solde & Mouvements

```go
balance, err := client.Wallet.Balance(ctx)

balance.Balance.Available // 45000 — dépensable immédiatement
balance.Balance.Held      //  5000 — en hold < 48 h
balance.Balance.Total     // 50000 — available + held

// Détail des holds en cours (issus des Cash-In récents)
for _, h := range balance.Holds {
	fmt.Printf("%.0f disponible le %s\n", h.Amount, h.AvailableAt)
}

// Statistiques du jour
if s := balance.StatsToday; s != nil {
	fmt.Printf("Cash-In : %d ops / %.0f\n", s.CashInCount, s.CashInVolume)
	fmt.Printf("Cash-Out: %d ops / %.0f\n", s.CashOutCount, s.CashOutVolume)
}

// Limites
if l := balance.Limits; l != nil {
	fmt.Printf("Plafond Cash-In/j : %.0f\n", l.DailyCashInLimit)
	fmt.Printf("Plafond Cash-Out/j: %.0f\n", l.DailyCashOutLimit)
}
```

> ⚠️ `balance.Available ≠ balance.Total` pendant 48 h après un Cash-In. **Les fonds en `held` ne peuvent pas être dépensés** — un Cash-Out qui les inclut échouera en `402`.

### Mouvements (crédits / débits)

```go
movements, err := client.Wallet.Movements(ctx, hrpay.WalletMovementsParams{
	Page:  1,
	Limit: 50,
	From:  "2026-06-01",
	To:    "2026-06-30",
})

for _, m := range movements.Data {
	fmt.Printf("[%s] %s %.0f %s → solde %.0f\n",
		m.CreatedAt, m.Type, m.Amount, m.Currency, m.BalanceAfter) // CREDIT | DEBIT
}
```

---

## Services VAS — Airtime, Data, Factures

> 🚧 **Les services VAS nécessitent un déploiement backend.** En attente, l'API renvoie `503 PROVIDER_NOT_CONFIGURED`. Le SDK expose déjà le contrat définitif — votre code n'aura pas à changer.

```go
_, err := client.Airtime.Recharge(ctx, params)
if errors.Is(err, hrpay.ErrProviderNotConfigured) {
	log.Println("service VAS pas encore déployé — réessayer plus tard")
}
```

### Airtime — Recharge téléphonique · commission 3 %

Opérateurs : `orange`, `mtn`, `camtel`. Champ **`phone`** (et non `phone_number`).

```go
resp, err := client.Airtime.Recharge(ctx, hrpay.AirtimeRechargeParams{
	Operator: hrpay.OperatorMtn,
	Phone:    "237680216505",
	Amount:   500,
})
fmt.Printf("%s — commission %.0f\n", resp.Reference, resp.Commission)
```

Recharge en lot (jusqu'à 500 destinataires) :

```go
batch, err := client.Airtime.Batch(ctx, hrpay.AirtimeBatchParams{
	Items: []hrpay.AirtimeBatchItem{
		{Phone: "237680216505", Operator: hrpay.OperatorMtn, Amount: 500},
		{Phone: "237655500393", Operator: hrpay.OperatorOrange, Amount: 1000},
	},
})
fmt.Printf("%d réussis / %d échoués\n", batch.Success, batch.Failed)
```

### Data — Forfaits internet · commission 3 %

Opérateurs : `mtn`, `orange`, `camtel`, `nexttel`.

```go
// Catalogue par opérateur
packages, err := client.Data.Packages(ctx, "mtn")
for _, p := range packages {
	fmt.Printf("%s — %.0f %s (%s)\n", p.Name, p.Amount, p.Currency, p.Validity)
}

// Envoi
resp, err := client.Data.Send(ctx, hrpay.DataSendParams{
	Operator: hrpay.OperatorMtn,
	Phone:    "237680216505",
	Amount:   1000,
})
```

### ENEO — Électricité · commission 1,5 %

Champ **`meter`** (et non `meter_number`). `CustomerPhone` est **optionnel** (reçu SMS).

```go
// Consulter une facture
invoice, err := client.Bills.Eneo.Invoice(ctx, "12345678")
fmt.Printf("%s — solde %.0f\n", invoice.Name, invoice.Balance)

// Prépayé → renvoie un token de recharge
resp, err := client.Bills.Eneo.Prepaid(ctx, hrpay.EneoPrepaidParams{
	Meter:         "12345678",
	Amount:        5000,
	CustomerPhone: "237680216505", // optionnel
})
fmt.Printf("Token de recharge : %s\n", resp.Token)

// Postpaid
resp, err = client.Bills.Eneo.Postpaid(ctx, hrpay.EneoPostpaidParams{
	Meter:  "12345678",
	Amount: 12000,
})
```

### CAMWATER — Eau · commission 1,5 %

```go
invoice, err := client.Bills.Camwater.Invoice(ctx, "CW123456")

resp, err := client.Bills.Camwater.Pay(ctx, hrpay.CamwaterPayParams{
	Meter:         "CW123456",
	Amount:        8500,
	CustomerPhone: "237680216505", // optionnel
})
```

### CanalPlus — Abonnement TV · commission 2 %

`DecoderNumber` : **8 à 12 chiffres** (validé côté SDK). `CustomerPhone` et `CustomerName` optionnels.

```go
resp, err := client.Bills.CanalPlus.Pay(ctx, hrpay.CanalPlusPayParams{
	DecoderNumber: "123456789",
	Amount:        10000,
	CustomerName:  "Jean Dupont",  // optionnel
	CustomerPhone: "237680216505", // optionnel
})
```

### Douanes DGD / SYDONIA · commission 1 %

```go
declaration, err := client.Bills.Customs.Get(ctx, "DGD-2026-001234")
fmt.Printf("%s — %.0f XAF\n", declaration.CustomerName, declaration.Amount)

resp, err := client.Bills.Customs.Pay(ctx, hrpay.CustomsPayParams{
	DeclarationRef: "DGD-2026-001234",
	Amount:         declaration.Amount, // montant exact de la déclaration
	CustomerName:   "SARL Import Export", // optionnel
	CustomerPhone:  "237680216505",       // optionnel
})
```

---

## Commissions revendeur

Chaque paiement VAS **crédite automatiquement** une commission sur votre wallet. Aucune facturation séparée.

| Service     | Commission par défaut | Constante                     |
| ----------- | ---------------------- | ----------------------------- |
| Airtime     | **3 %**          | `hrpay.VasServiceAirtime`   |
| Data        | **3 %**          | `hrpay.VasServiceData`      |
| ENEO        | **1,5 %**        | `hrpay.VasServiceEneo`      |
| CAMWATER    | **1,5 %**        | `hrpay.VasServiceCamwater`  |
| CanalPlus   | **2 %**          | `hrpay.VasServiceCanalPlus` |
| Douanes DGD | **1 %**          | `hrpay.VasServiceCustoms`   |

```go
// Grille tarifaire réelle de votre compte (temps réel)
rates, err := client.Commissions.Rates(ctx)

// Barème par défaut, sans appel réseau
fmt.Println(hrpay.DefaultCommissionRates[hrpay.VasServiceAirtime]) // 3.0

// Historique filtrable
history, err := client.Commissions.History(ctx, hrpay.CommissionHistoryParams{
	Service: hrpay.VasServiceEneo,
	From:    "2026-06-01",
	To:      "2026-06-30",
})

// Résumé par service sur une période
summary, err := client.Commissions.Summary(ctx, "2026-06-01", "2026-06-30")
fmt.Printf("Total : %.0f %s\n", summary.TotalCommission, summary.Currency)
for svc, amount := range summary.ByService {
	fmt.Printf("  %s : %.0f\n", svc, amount)
}
```

---

## Payroll — Paiements de masse

Workflow : **import → exécution → rapport**.

```go
// 1. Importer le batch → batch_id
imported, err := client.Payroll.Import(ctx, hrpay.PayrollImportParams{
	Label:    "Salaires Juin 2026",
	Currency: hrpay.CurrencyXaf,
	Recipients: []hrpay.PayrollRecipient{
		{PhoneNumber: "237670000001", Operator: hrpay.OperatorMtn, Amount: 150000, Name: "Jean Dupont"},
		{PhoneNumber: "237655000002", Operator: hrpay.OperatorOrange, Amount: 200000, Name: "Marie Martin"},
	},
})
if err != nil {
	log.Fatal(err)
}
batchID := imported.BatchID

// 2. Exécuter — déclenche les envois
batch, err := client.Payroll.Execute(ctx, batchID)
fmt.Printf("%s : %d bénéficiaires, %.0f total\n",
	batch.Status, batch.TotalRecipients, batch.TotalAmount)

// 3. Suivre le statut
for {
	batch, err = client.Payroll.Status(ctx, batchID)
	if err != nil {
		log.Fatal(err)
	}
	if batch.Status == "COMPLETED" || batch.Status == "PARTIALLY_COMPLETED" || batch.Status == "FAILED" {
		break
	}
	time.Sleep(15 * time.Second)
}

// 4. Rapport détaillé par bénéficiaire
report, err := client.Payroll.Report(ctx, batchID)
fmt.Printf("%d réussis / %d échoués — frais %.0f\n",
	report.Summary.Success, report.Summary.Failed, report.Summary.TotalFees)

for _, item := range report.Items {
	if item.Status == hrpay.StatusFailed {
		fmt.Printf("❌ %s (%s) : %s\n", item.Name, item.PhoneNumber, item.Error)
	}
}

// Lister tous les batches
batches, err := client.Payroll.List(ctx, hrpay.PayrollListParams{Page: 1, Limit: 20})
```

Import possible aussi via `FileBase64` ou `CSVData` au lieu de `Recipients`.

---

## Cartes virtuelles

Cartes Visa/Mastercard prépayées, alimentées depuis votre wallet XAF. Provider : Cartevo.

```go
// Créer
card, err := client.Cards.Create(ctx, hrpay.VirtualCardCreateParams{
	Label:    "Carte Marketing",
	Currency: hrpay.CurrencyXaf,
	Amount:   50000, // solde initial
})

// Lister / détailler
cards, err := client.Cards.List(ctx)
card, err = client.Cards.Get(ctx, card.ID)

// Recharger depuis le wallet
card, err = client.Cards.Topup(ctx, card.ID, 25000)

// Geler / dégeler — toutes ces méthodes renvoient la carte mise à jour
card, err = client.Cards.Freeze(ctx, card.ID)
card, err = client.Cards.Unfreeze(ctx, card.ID)

// Annuler — IRRÉVERSIBLE
card, err = client.Cards.Cancel(ctx, card.ID)

fmt.Println(card.Status) // ACTIVE | FROZEN | CANCELLED
```

---

## Liens de paiement

```go
link, err := client.PaymentLinks.Create(ctx, hrpay.PaymentLinkCreateParams{
	Amount:      25000,
	Currency:    hrpay.CurrencyXaf,
	Description: "Facture #2026-042",
	ExpiresAt:   time.Now().Add(72 * time.Hour).Format(time.RFC3339),
})
fmt.Println(link.URL) // à partager avec le client

links, err := client.PaymentLinks.List(ctx, hrpay.PaymentLinkListParams{Page: 1, Limit: 20})
```

---

## Webhooks

Configurez votre URL : **Dashboard → Paramètres → Webhooks**.

| Événement           | Constante                       | Déclencheur                     |
| --------------------- | ------------------------------- | -------------------------------- |
| `payment.succeeded` | `hrpay.EventPaymentSucceeded` | Cash-In/Out passé à`SUCCESS` |
| `payment.failed`    | `hrpay.EventPaymentFailed`    | Cash-In/Out passé à`FAILED`  |
| `payment.hold`      | `hrpay.EventPaymentHold`      | Transaction bloquée par l'AML   |
| `payment.refunded`  | `hrpay.EventPaymentRefunded`  | Remboursement effectué          |

### Vérification de signature

Chaque requête porte l'en-tête `X-Hub-Signature: sha256=<hmac>`. Le SDK vérifie le HMAC-SHA256 du **corps brut** en temps constant.

> ⚠️ **Utilisez impérativement le corps brut** (`io.ReadAll(r.Body)`), **avant** tout décodage JSON. Ré-encoder le JSON change les octets et invalide la signature.

### Handler HTTP complet et fonctionnel

```go
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/hr-skills/hrpay"
)

type paymentPayload struct {
	Reference string                 `json:"reference"`
	Status    string                 `json:"status"`
	Amount    float64                `json:"amount"`
	NetAmount float64                `json:"net_amount"`
	Currency  string                 `json:"currency"`
	Metadata  map[string]interface{} `json:"metadata"`
}

func webhookHandler(client *hrpay.Client, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Lire le corps BRUT — indispensable pour la signature
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 2. Vérifier la signature et décoder
		signature := r.Header.Get("X-Hub-Signature") // "sha256=..."
		event, err := client.Webhooks.ConstructEvent(string(body), signature, secret)
		if err != nil {
			log.Printf("signature invalide : %v", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		// 3. Répondre 200 RAPIDEMENT, traiter en asynchrone.
		//    Un traitement lent provoque des retries côté HR-Skills Pay.
		w.WriteHeader(http.StatusOK)

		go func() {
			var p paymentPayload
			if err := json.Unmarshal(event.Data, &p); err != nil {
				log.Printf("payload illisible : %v", err)
				return
			}

			switch event.Type {
			case hrpay.EventPaymentSucceeded:
				// ⚠️ Idempotence : le même événement peut arriver plusieurs fois.
				// Déduplication sur event.ID ou p.Reference.
				log.Printf("✅ %s confirmé — net %.0f %s", p.Reference, p.NetAmount, p.Currency)

			case hrpay.EventPaymentFailed:
				log.Printf("❌ %s échoué", p.Reference)

			case hrpay.EventPaymentHold:
				log.Printf("⏸️  %s bloqué par l'AML", p.Reference)

			case hrpay.EventPaymentRefunded:
				log.Printf("↩️  %s remboursé", p.Reference)
			}
		}()
	}
}

func main() {
	client, _ := hrpay.NewClient(
		hrpay.WithAPIKeys(os.Getenv("HRPAY_PK"), os.Getenv("HRPAY_SK")),
	)
	http.HandleFunc("/webhooks/hrpay", webhookHandler(client, os.Getenv("HRPAY_WEBHOOK_SECRET")))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Vérification seule, sans décodage :

```go
ok := client.Webhooks.VerifySignature(string(body), signature, secret)
```

Le SDK accepte l'en-tête préfixé (`sha256=<hmac>`, format officiel) **et** un digest hexadécimal nu.

---

## Opérateurs & Pays

| Pays           | Code          | Devise | Opérateurs                        |
| -------------- | ------------- | ------ | ---------------------------------- |
| Cameroun       | `CountryCm` | XAF    | mtn · orange · camtel            |
| Sénégal      | `CountrySn` | XOF    | orange · wave · free · expresso |
| Côte d'Ivoire | `CountryCi` | XOF    | orange · mtn · moov · wave      |
| Gabon          | `CountryGa` | XAF    | airtel · moov                     |
| RD Congo       | `CountryCd` | CDF    | airtel · orange · mpesa          |
| Mali           | `CountryMl` | XOF    | orange · moov                     |
| Burkina Faso   | `CountryBf` | XOF    | orange · moov · coris            |
| Togo           | `CountryTg` | XOF    | tmoney · flooz                    |
| Bénin         | `CountryBj` | XOF    | mtn · moov                        |
| Guinée        | `CountryGn` | GNF    | orange · mtn · afrimoney         |
| Gambie         | `CountryGm` | GMD    | afrimoney · qmoney                |

**Constantes opérateur :** `OperatorMtn`, `OperatorOrange`, `OperatorMoov`, `OperatorAirtel`, `OperatorMpesa`, `OperatorWave`, `OperatorFree`, `OperatorTmoney`, `OperatorAfrimoney`, `OperatorCamtel`, `OperatorNexttel`, `OperatorCoris`, `OperatorExpresso`, `OperatorFlooz`, `OperatorQmoney`.

**Devises :** `CurrencyXaf`, `CurrencyXof`, `CurrencyCdf`, `CurrencyGnf`, `CurrencyGmd` (+ `CurrencyUsd`, `CurrencyEur` pour les cartes).

La devise locale est déduite du pays quand elle est omise :

```go
hrpay.CurrencyForCountry[hrpay.CountrySn] // CurrencyXof
```

---

## Gestion des erreurs

Format renvoyé par l'API : `{"error": "CODE", "message": "...", "details": {...}}`

| HTTP | Code                            | Sentinelle Go                    |
| ---- | ------------------------------- | -------------------------------- |
| 400  | `VALIDATION_ERROR`            | `ErrValidation`                |
| 400  | `MISSING_REQUIRED_FIELD`      | `ErrMissingRequiredField`      |
| 401  | `MISSING_API_KEY`             | `ErrMissingAPIKey`             |
| 401  | `INVALID_API_KEY`             | `ErrInvalidAPIKey`             |
| 401  | `MISSING_TRANSACTION_TOKEN`   | `ErrMissingTransactionToken`   |
| 401  | `INVALID_TRANSACTION_TOKEN`   | `ErrInvalidTransactionToken`   |
| 402  | `WALLET_BALANCE_INSUFFICIENT` | `ErrWalletBalanceInsufficient` |
| 403  | `KYC_NOT_APPROVED`            | `ErrKycNotApproved`            |
| 403  | `WALLET_FROZEN`               | `ErrWalletFrozen`              |
| 409  | `IDEMPOTENCY_KEY_CONFLICT`    | `ErrIdempotencyKeyConflict`    |
| 422  | `OPERATOR_NOT_AVAILABLE`      | `ErrOperatorNotAvailable`      |
| 422  | `CURRENCY_MISMATCH`           | `ErrCurrencyMismatch`          |
| 429  | `RATE_LIMIT_EXCEEDED`         | `ErrRateLimitExceeded`         |
| 500  | `INTERNAL_ERROR`              | `ErrInternalError`             |
| 503  | `PROVIDER_NOT_CONFIGURED`     | `ErrProviderNotConfigured`     |
| 504  | `PROVIDER_TIMEOUT`            | `ErrProviderTimeout`           |

### Détecter un cas métier avec `errors.Is`

```go
tx, err := client.CashOut.MobileMoney(ctx, params)
if err != nil {
	switch {
	case errors.Is(err, hrpay.ErrWalletBalanceInsufficient):
		fmt.Println("Approvisionnez le compte.")
	case errors.Is(err, hrpay.ErrKycNotApproved):
		fmt.Println("KYC non approuvé — clés LIVE bloquées.")
	case errors.Is(err, hrpay.ErrWalletFrozen):
		fmt.Println("Wallet gelé par l'administrateur.")
	case errors.Is(err, hrpay.ErrOperatorNotAvailable):
		fmt.Println("Opérateur indisponible dans ce pays.")
	case errors.Is(err, hrpay.ErrRateLimitExceeded):
		fmt.Println("Trop de requêtes, ralentissez.")
	case errors.Is(err, hrpay.ErrProviderNotConfigured):
		fmt.Println("Service VAS pas encore déployé.")
	}
}
```

### Exploiter `details` avec `errors.As`

Le champ `details` porte le contexte (ex. `shortfall` sur un `402`) :

```go
var walletErr *hrpay.WalletError
if errors.As(err, &walletErr) {
	fmt.Printf("Code    : %s\n", walletErr.Code)       // WALLET_BALANCE_INSUFFICIENT
	fmt.Printf("HTTP    : %d\n", walletErr.StatusCode) // 402
	fmt.Printf("Manque  : %v\n", walletErr.Details["shortfall"])
}
```

### Erreurs de validation locale (avant tout appel réseau)

```go
var valErr *hrpay.ValidationError
if errors.As(err, &valErr) {
	for _, issue := range valErr.Issues {
		fmt.Printf("- %s : %s\n", issue.Field, issue.Message)
	}
}
```

Le SDK valide côté client : montant minimum (100 en Cash-In), format des numéros, longueur du décodeur CanalPlus (8-12 chiffres), `limit ≤ 100`…

### Types d'erreurs structurés

`*ValidationError` · `*AuthenticationError` · `*WalletError` · `*ConflictError` · `*RateLimitError` · `*ApiError` · `*NetworkError` · `*TimeoutError` · `*CircuitBreakerOpenError` · `*WebhookSignatureError` · `*UnknownError`

> Les anciennes sentinelles (`ErrInsufficientBalance`, `ErrKycRequired`, `ErrDuplicateTransaction`…) restent disponibles en alias des codes canoniques.

---

## Configuration & Résilience

```go
client, err := hrpay.NewClient(
	hrpay.WithAPIKeys(publicKey, secretKey),
	hrpay.WithBaseURL("https://api.hrskills-pay.com"), // optionnel
	hrpay.WithTimeout(15*time.Second),
	hrpay.WithMaxRetries(5),
	hrpay.WithThrottle(100*time.Millisecond), // débit max
	hrpay.WithTokenCache(hrpay.NewFileTokenCache(".hrpay-token.json")),
	hrpay.WithLogger(hrpay.LoggerConfig{
		Requests:  true,
		Responses: true,
		Errors:    true,
		Log:       func(msg string) { myLogger.Info(msg) },
	}),
	hrpay.WithOnError(func(err error, mctx *hrpay.MiddlewareContext) {
		sentry.CaptureException(err)
	}),
)
```

Fonctionnalités intégrées :

- **Retry intelligent** — backoff exponentiel sur `429`/`5xx`, respect de `Retry-After`.
- **Circuit breaker** — ouvre après 5 échecs, se referme après 15 s (`*CircuitBreakerOpenError`).
- **Idempotence automatique** — `Idempotency-Key` (UUID v4) sur `POST`/`PUT`/`PATCH`.
- **Panic recovery** — un `panic` dans un middleware est capturé et renvoyé en `*UnknownError`.
- **Rédaction des secrets** — les clés sont masquées dans les logs.
- **Rafraîchissement de token thread-safe** — protégé par mutex.

### Middlewares personnalisés (style Koa)

```go
client.Use(func(ctx context.Context, mctx *hrpay.MiddlewareContext, next hrpay.Next) error {
	start := time.Now()
	mctx.Headers["X-Request-ID"] = uuid.NewString()

	err := next(ctx, mctx) // suite de la chaîne + requête HTTP

	if mctx.Response != nil {
		metrics.Observe(mctx.Method, mctx.Response.StatusCode, time.Since(start))
	}
	return err
})
```

---

## Tests

```bash
go test ./...                    # suite complète
go test -v ./...                 # détail par test
go test -race ./...              # détection de data races
go test -run TestSandbox ./...   # comportement sandbox (parité des montants)
```

Le SDK embarque une suite de **tests de conformité** ([`conformance_test.go`](conformance_test.go)) qui verrouille le contrat documenté : parité sandbox, champ `fee`, `holds[]`, format de signature webhook `sha256=`, codes d'erreur, champ `meter` (et non `meter_number`), dérivation de la devise depuis le pays, `limit ≤ 100`, montant minimum de 100…

---

## Licence

MIT © [HR-Skills Pay](https://hrskills-pay.com)
# hrpay
