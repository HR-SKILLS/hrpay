# HR-Skills Pay Go SDK

> SDK Go officiel pour [HR-Skills Pay](https://hrskills-pay.com) — Mobile Money, Portefeuilles, Airtime, Factures, Payroll, Cartes Virtuelles et plus.

Infrastructure de paiement B2B pour l'Afrique — **16 pays** : Cameroun, Côte d'Ivoire, Sénégal, Gabon, RD Congo, Mali, Burkina Faso, Togo, Bénin, Guinée, Gambie et plus.

|                    |                                                                |
| ------------------ | -------------------------------------------------------------- |
| **Base URL** | `https://api.hrskills-pay.com`                               |
| **Frais**    | 2 % flat par défaut (Cash-In et Cash-Out) — **non garanti**, des taux négociés existent par marchand. Consultez toujours `client.Fees.List(ctx)`. |
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
16. [Pièges connus](#pièges-connus)
17. [Gestion des erreurs](#gestion-des-erreurs)
18. [Configuration &amp; Résilience](#configuration--résilience)

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
	fmt.Printf("Frais       : %.0f (%.1f%%)\n", tx.Fees, tx.FeePercent) // 75 (1.5%)
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

|                            | Rôle                                                                                                          |
| -------------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Clé A** (`hrsk_pk_...`) | Envoyée dans `Authorization: Bearer` **uniquement** pour obtenir le Transaction Token.                        |
| **Clé B** (`hrsk_sk_...`) | Envoyée dans `Authorization: Bearer` sur **tous les appels de paiement/ressource** (payin, payout, refund, cartes…). |
| **Transaction Token**      | Ajouté en `X-Transaction-Token` sur ces mêmes appels. JWT HMAC-SHA256, TTL **45 min** (`expires_in: 2700`)     |

> ⚠️ **Piège fréquent** : c'est bien la **Clé B** (secrète) qui authentifie les appels de paiement — la Clé A ne sert qu'à l'échange initial du Transaction Token. `GET /v1/wallet/balance` est un cas à part : Clé B **seule**, sans Transaction Token. Le SDK gère cette distinction pour vous automatiquement — vous n'avez jamais à choisir la clé à envoyer vous-même.

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
| `Country`     | Country  | ✅     | ISO 3166-1 alpha-2.**Jamais déduit ni par défaut** — la devise est toujours dérivée du pays côté serveur, donc un pays incorrect enverrait la demande au mauvais endroit. |
| `Amount`      | float64  | ✅     | Devise locale.**Minimum 100.** Pas de décimales.    |
| `Reference`   | string   | —     | Votre référence unique. Auto-générée (`ref_...`) si omise. Réutiliser une référence déjà utilisée renvoie `409 DUPLICATE_REFERENCE`. |
| `Currency`    | Currency | —     | Défaut :**déduite du pays**. Si fournie, doit correspondre exactement (sinon `422 CURRENCY_MISMATCH`). |
| `Description` | string   | —     | Motif affiché au client sur son téléphone               |
| `Metadata`    | map      | —     | Données libres retournées dans le webhook                |

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

Réponse (`202`) — un `*hrpay.Transaction`, tous les champs sont typés :

```go
tx.TransactionID // "a959b6ca-a5e6-4485-8f92-0747a574b54e"
tx.ExternalID    // "COLL-1757600000-a1b2c3d4"
tx.Reference     // "ref_d5b40df948dc52cc"  ← à stocker pour le suivi
tx.Status        // "PENDING" (toujours, initialement)
tx.Direction     // "CASHIN"
tx.Amount        // 5000
tx.Fees          // 75      ← 1,5 % du montant
tx.FeePercent    // 1.5
tx.NetAmount     // 4925    ← crédité après confirmation
tx.Currency      // "XAF"
tx.OtpRequired   // false (true pour Orange en CI/SN, Wligdicash au BF)
tx.Provider      // "CARTEVO"  ← informatif, ne pas coder de logique dessus
tx.ProviderRef   // "sandbox-a1b2c3d4"
tx.InitiatedAt   // "2026-06-07T17:00:59Z"
tx.PendingAction // "GET /v1/payments/ref_d5b40df948dc52cc"
```

Une fois la transaction résolue, `Transactions.Status`/`.Get` ajoutent `CompletedAt`, `ErrorMessage`, `ErrorCode`, `FailureReasonCategory`, et `WalletBalanceBefore`/`WalletBalanceAfter`.

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

### Remboursement

```go
tx, err := client.Transactions.Refund(ctx, "ref_d5b40df948dc52cc")
// déclenche le webhook payment.refunded ; tx.Status == "REFUNDED"
```

Un `Idempotency-Key` est requis (généré automatiquement) ; pour rejouer un retry en toute sécurité après une coupure réseau, fournissez la même clé explicitement :

```go
ctx = hrpay.WithIdempotencyKey(ctx, "refund-commande-42891")
tx, err := client.Transactions.Refund(ctx, "commande-42891")
```

> Ne réutilisez **jamais** une `Idempotency-Key` pour une opération différente : la réponse de la première tentative est rejouée telle quelle pendant 24 h, sans vérification que le corps de la requête correspond.

### Frais applicables

Le taux par défaut (2 % flat) n'est **pas garanti** — des taux négociés existent par marchand. Vérifiez toujours le barème réel :

```go
fees, err := client.Fees.List(ctx)
for _, f := range fees.Fees {
	fmt.Printf("%s %s: %.1f%% (défaut plateforme: %.1f%%)\n", f.Country, f.Direction, f.FeePct, f.DefaultFeePct)
}
```

---

## Solde & Mouvements

> `Wallet.Balance` s'authentifie avec la **Clé B seule** — pas de Transaction Token sur cet appel. C'est géré automatiquement par le SDK.

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

Cartes Visa/Mastercard prépayées en **USD**, émises pour vos propres bénéficiaires (employés, clients, prestataires). Provider : Cartevo. Trois étapes **obligatoires**, dans cet ordre : enrôler un porteur (KYC) → financer le wallet USD dédié aux cartes → émettre la carte.

`client.Cards` regroupe trois sous-services, à l'image de `client.Bills` :

- `client.Cards.Customers` — KYC des porteurs
- `client.Cards.Wallet` — wallet USD dédié, cotation FX, tarification
- `client.Cards.Virtual` — cycle de vie de la carte elle-même

> ⚠️ **Casse particulière** : contrairement au reste de l'API (`snake_case` partout), l'objet `card` renvoyé par `Create`/`Get`/`List` est sérialisé par le serveur en **PascalCase** (`ID`, `CustomerID`, `CardNetwork`…). Le SDK gère cela pour vous — `hrpay.VirtualCard` a des tags JSON PascalCase pour ce type précis, c'est intentionnel, ne le "corrigez" pas.

### 1. Enrôler un porteur (KYC)

```go
customer, err := client.Cards.Customers.Create(ctx, hrpay.CardCustomerCreateParams{
	FirstName: "Jean", LastName: "Dupont", Email: "jean.dupont@client.cm",
	Country: "Cameroon", CountryIsoCode: "CM", CountryPhoneCode: "+237",
	PhoneNumber: "690001234", // local uniquement, sans l'indicatif
	Street: "Rue 1.234, Bonanjo", City: "Douala", State: "Littoral", PostalCode: "00237",
	IdentificationNumber: "123456789",
	IDDocumentType:       hrpay.IDDocumentNIN, // NIN | PASSPORT | VOTERS_CARD | DRIVERS_LICENSE
	DateOfBirth:          "1990-04-12",         // porteur majeur obligatoire
	IDDocumentFront:      "data:image/jpeg;base64,...",
	IDDocumentBack:       "data:image/jpeg;base64,...",
})
// customer.KYCStatus == "PENDING_REVIEW" — un administrateur HR-Skills Pay
// doit encore approuver le dossier ; ce n'est jamais instantané et il
// n'existe aucun appel API pour accélérer cette revue.
```

Attendez le statut `ENROLLED` (seul statut permettant l'émission d'une carte) :

```go
customer, err = client.Cards.Customers.PollEnrollment(ctx, customer.ID, hrpay.PollOptions{
	// Intervalle par défaut : 5 minutes (hrpay.DefaultKYCPollInterval) — la
	// revue est humaine, ne pollez pas toutes les quelques secondes.
	OnStatus: func(status string, attempt int) { log.Printf("KYC: %s", status) },
})
```

### 2. Financer le wallet USD cartes

```go
overview, err := client.Cards.Wallet.Get(ctx) // solde + tarification actuelle
res, err := client.Cards.Wallet.Fund(ctx, hrpay.CardWalletFundParams{AmountUSD: 10})
// res.USDBalance — nouveau solde du wallet cartes
```

`source_currency` n'accepte que `XAF` aujourd'hui — un marchand dont le wallet principal est en XOF/GNF/GMD/CDF ne peut pas encore utiliser les cartes.

### 3. Émettre, gérer, terminer une carte

```go
res, err := client.Cards.Virtual.Create(ctx, hrpay.VirtualCardCreateParams{
	CustomerID: customer.ID,
	Brand:      hrpay.CardBrandVisa, // ou "MC" (normalisé en MASTERCARD)
	Amount:     20,                  // financement initial en USD
	Label:      "Marketing Ads",
})
if res.Pending {
	// Émission ambiguë côté fournisseur — réconciliée plus tard
	// automatiquement (webhook card.created ou worker interne).
	fmt.Println("carte en attente de confirmation:", res.CardID)
} else {
	card := res.Card
	fmt.Println(card.ID, card.Status, card.ProviderData.MaskedPan)
}

// Révéler le PAN/CVV en clair — audité côté serveur, jamais journalisé.
// Ne journalisez et ne persistez JAMAIS ce bloc côté application non plus.
detail, err := client.Cards.Virtual.Get(ctx, card.ID, hrpay.VirtualCardGetParams{Reveal: true})
fmt.Println(detail.Sensitive.Number, detail.Sensitive.CVV)

// Recharger / retirer
topup, err := client.Cards.Virtual.Topup(ctx, card.ID, hrpay.VirtualCardTopupParams{Amount: 50})
withdraw, err := client.Cards.Virtual.Withdraw(ctx, card.ID, hrpay.VirtualCardWithdrawParams{Amount: 30})

// Geler / dégeler
_, err = client.Cards.Virtual.Freeze(ctx, card.ID, hrpay.VirtualCardFreezeParams{})
_, err = client.Cards.Virtual.Unfreeze(ctx, card.ID, hrpay.VirtualCardUnfreezeParams{})

// Terminer — IRRÉVERSIBLE, solde résiduel recrédité au wallet cartes
result, err := client.Cards.Virtual.Terminate(ctx, card.ID)
fmt.Println(result.Status, result.Refunded)
```

### Historique des transactions carte

```go
txs, err := client.Cards.Virtual.Transactions(ctx, card.ID, hrpay.VirtualCardTransactionsParams{Limit: 50})
```

> ⚠️ **Pagination 0-indexée pour une carte live** (liée à Cartevo) : `Page: 0` est la première page — contrairement au reste de la plateforme. Une boucle qui commence à `Page: 1` saute la vraie première page.

### Sandbox

Émulation 100 % locale, aucun appel réseau réel vers Cartevo : PAN de test fixe `4111111111111111` / CVV `123`, taux de change fixe 630 XAF/USD, et **aucun webhook** n'est jamais déclenché pour une ressource `is_test: true`.

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

16 pays couverts pour le Mobile Money (cashin/cashout). CM est activé par défaut pour tout marchand ; les autres pays doivent être **explicitement activés** pour votre compte (sinon `403 COUNTRY_NOT_ACTIVATED`).

| Pays                    | Code          | Devise | Opérateurs                       | OTP requis            |
| ----------------------- | ------------- | ------ | --------------------------------- | ---------------------- |
| Cameroun                | `CountryCm` | XAF    | mtn · orange                    | —                      |
| Gabon                   | `CountryGa` | XAF    | airtel · moov                   | —                      |
| Congo Brazzaville       | `CountryCg` | XAF    | airtel · mtn                    | —                      |
| Tchad                   | `CountryTd` | XAF    | airtel · moov                   | —                      |
| Rép. Centrafricaine    | `CountryCf` | XAF    | orange                            | —                      |
| Côte d'Ivoire          | `CountryCi` | XOF    | moov · mtn · orange · wave    | orange                 |
| Sénégal               | `CountrySn` | XOF    | expresso · free · orange · wave | orange                 |
| Mali                    | `CountryMl` | XOF    | moov · orange                   | —                      |
| Burkina Faso            | `CountryBf` | XOF    | moov · orange · wligdicash    | orange, wligdicash     |
| Togo                    | `CountryTg` | XOF    | moov · tmoney                   | —                      |
| Bénin                  | `CountryBj` | XOF    | moov · mtn · celtiis · coris  | —                      |
| Niger                   | `CountryNe` | XOF    | airtel                            | —                      |
| Guinée-Bissau          | `CountryGw` | XOF    | orange                            | —                      |
| RD Congo                | `CountryCd` | CDF    | airtel · mpesa · orange · afrimoney | —                 |
| Guinée Conakry         | `CountryGn` | GNF    | mtn · orange                    | —                      |
| Gambie                  | `CountryGm` | GMD    | afrimoney                         | —                      |

**Constantes opérateur (Mobile Money) :** `OperatorMtn`, `OperatorOrange`, `OperatorMoov`, `OperatorAirtel`, `OperatorMpesa`, `OperatorWave`, `OperatorFree`, `OperatorTmoney`, `OperatorAfrimoney`, `OperatorWligdicash`, `OperatorCeltiis`, `OperatorCoris`, `OperatorExpresso`. (`OperatorCamtel`, `OperatorNexttel`, `OperatorFlooz`, `OperatorQmoney` existent aussi mais sont réservés aux domaines Airtime/Data/Payroll — ce ne sont **pas** des opérateurs Mobile Money valides.)

**Devises :** `CurrencyXaf`, `CurrencyXof`, `CurrencyCdf`, `CurrencyGnf`, `CurrencyGmd` (+ `CurrencyUsd`, `CurrencyEur` pour les cartes).

```go
hrpay.CurrencyForCountry[hrpay.CountrySn]                // CurrencyXof — devise locale déduite du pays
hrpay.MobileMoneyOperatorsByCountry[hrpay.CountryCi]     // opérateurs valides pour ce pays (référence/UX)
hrpay.OtpRequiredOperators[hrpay.CountryCi]              // opérateurs exigeant une confirmation OTP
```

> Ce tableau et ces maps documentent l'état du catalogue au moment de la rédaction — ils ne sont **pas** une garantie contractuelle et peuvent évoluer côté plateforme sans préavis (la source vivante, `GET /v1/countries/supported`, s'authentifie par JWT tableau de bord et n'est pas exposée par ce SDK). Ne construisez pas de blocage strict dessus.

---

## Pièges connus

- **`GET /v1/countries/supported`** (catalogue pays/opérateurs par marchand, avec statut d'activation) n'est **pas** exposé par ce SDK : il s'authentifie par session JWT tableau de bord, un mode entièrement différent du couple Clé API + Transaction Token utilisé partout ailleurs ici. Consultez-le depuis le tableau de bord, ou utilisez `hrpay.MobileMoneyOperatorsByCountry` comme référence non contractuelle.
- **`POST /v1/transfers/express-union` et `POST /v1/transfers/yoome`** existent côté API mais sont aujourd'hui des stubs purs (aucun appel fournisseur réel, réponse `PENDING` factice) — ce SDK n'expose volontairement **aucune** méthode pour ces routes.
- **`GET /v1/operators` et `GET /v1/services/catalogue`** sont legacy/potentiellement obsolètes côté API — préférez le tableau `MobileMoneyOperatorsByCountry` de ce SDK (lui-même non contractuel, voir ci-dessus) plutôt que ces endpoints.
- La méthode historique `CashIn.Initiate` (`/v1/payments/initiate`) a été retirée — elle ne correspondait à aucune route documentée. Utilisez `CashIn.MobileMoney`.

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
| 402  | `WALLET_NOT_FOUND`            | `ErrWalletNotFound`            |
| 403  | `KYC_NOT_APPROVED`            | `ErrKycNotApproved`            |
| 403  | `WALLET_FROZEN`               | `ErrWalletFrozen`              |
| 403  | `COUNTRY_NOT_ACTIVATED`       | `ErrCountryNotActivated`       |
| 403  | `SANDBOX_KEY_REQUIRED`        | `ErrSandboxKeyRequired`        |
| 403  | `SANDBOX_PATH_REQUIRED`       | `ErrSandboxPathRequired`       |
| 409  | `IDEMPOTENCY_KEY_CONFLICT`    | `ErrIdempotencyKeyConflict`    |
| 409  | `DUPLICATE_REFERENCE`         | `ErrDuplicateReference`        |
| 422  | `INVALID_AMOUNT`              | `ErrInvalidAmount`             |
| 422  | `INVALID_OPERATOR_COUNTRY`    | `ErrInvalidOperatorCountry`    |
| 422  | `MISSING_PHONE`               | `ErrMissingPhone`              |
| 422  | `INVALID_PHONE`               | `ErrInvalidPhone`              |
| 422  | `OPERATOR_NOT_AVAILABLE`      | `ErrOperatorNotAvailable`      |
| 422  | `CURRENCY_MISMATCH`           | `ErrCurrencyMismatch`          |
| 422  | `AMOUNT_EXCEEDS_LIMIT`        | `ErrAmountExceedsLimit`        |
| 422  | `DAILY_LIMIT_EXCEEDED`        | `ErrDailyLimitExceeded`        |
| 422  | `cashout_refused`             | `ErrCashoutRefused`            |
| 429  | `RATE_LIMIT_EXCEEDED`         | `ErrRateLimitExceeded`         |
| 429  | `PLAN_TX_LIMIT_REACHED`       | `ErrPlanTxLimitReached`        |
| 429  | `TOO_MANY_REQUESTS`           | `ErrTooManyRequests`           |
| 500  | `INTERNAL_ERROR`              | `ErrInternalError`             |
| 503  | `PROVIDER_NOT_CONFIGURED`     | `ErrProviderNotConfigured`     |
| 503  | `PROVIDER_UNAVAILABLE`        | `ErrProviderUnavailable`       |
| 504  | `PROVIDER_TIMEOUT`            | `ErrProviderTimeout`           |

`ErrCashoutRefused` couvre tous les refus opérateur sur un Cash-Out (client rejeté, PIN invalide, fonds bénéficiaire insuffisants…) — lisez `err.Message` (déjà humanisé) plutôt que de tenter de parser le code numérique brut (703201, 703202… voir les constantes `CashoutRefusalXxx` pour référence).

**Cartes virtuelles** — enveloppe distincte `{"code": "...", "message": "..."}` (**sans** champ `success`), gérée automatiquement par `ParseApiError`. Sentinelles notables : `ErrCustomerNotEnrolled`, `ErrEnvironmentMismatch`, `ErrCardNotActive`, `ErrCardNotEligible`, `ErrCardAlreadyTerminated`, `ErrCardNotFound`, `ErrCardCustomerNotFound`, `ErrCardCustomerExists`, `ErrRequestInProgress`, `ErrMaxCardsReached`, `ErrCardBalanceInsufficient`, `ErrCardCreationRejected`, `ErrCardProviderUnavailable`, `ErrCardRateUnavailable`, `ErrIdempotencyUnavailable`, `ErrPlanFeatureNotAvailable`. Note : le code `insufficient_balance` (422, wallet USD cartes) partage intentionnellement la sentinelle `ErrWalletBalanceInsufficient` avec le cas Mobile Money (402) — distinguez via `StatusCode` ou le type Go concret (`*ValidationError` vs `*WalletError`), pas via la sentinelle seule.

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
