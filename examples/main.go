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

// Exécution :
//
//	export HRPAY_PK="hrsk_pk_test_..."
//	export HRPAY_SK="hrsk_sk_test_..."
//	go run ./examples
//
// Ne jamais coder les clés en dur : la Clé B (hrsk_sk_...) est un secret.
func main() {
	fmt.Println("=== HR-Skills Pay Go SDK Example ===")

	publicKey := os.Getenv("HRPAY_PK")
	secretKey := os.Getenv("HRPAY_SK")
	if publicKey == "" || secretKey == "" {
		log.Fatal("HRPAY_PK et HRPAY_SK doivent être définis dans l'environnement")
	}

	client, err := hrpay.NewClient(
		hrpay.WithAPIKeys(publicKey, secretKey),
		hrpay.WithTimeout(15*time.Second),
		hrpay.WithMaxRetries(3),
		hrpay.WithLogger(hrpay.LoggerConfig{
			Requests:  true,
			Responses: true,
			Errors:    true,
		}),
	)
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	ctx := context.Background()

	// 1. Solde du portefeuille
	fmt.Println("\n-> Fetching Wallet Balance...")
	balance, err := client.Wallet.Balance(ctx)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
	} else {
		fmt.Printf("   ✅ Available: %.0f %s (held: %.0f)\n",
			balance.Balance.Available, balance.Currency, balance.Balance.Held)
		for _, h := range balance.Holds {
			fmt.Printf("      hold de %.0f disponible le %s\n", h.Amount, h.AvailableAt)
		}
	}

	// L'environnement est déterminé par la clé, pas par l'URL.
	fmt.Printf("\n-> Environnement : %s (marchand %s)\n",
		client.Auth.Environment(), client.Auth.MerchantID())

	// 2. Cash-In. En sandbox : montant PAIR → SUCCESS, IMPAIR → FAILED.
	fmt.Println("\n-> Initiating Cash-In (5000 XAF — pair, donc SUCCESS en sandbox)...")
	tx, err := client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    hrpay.OperatorOrange,
		Country:     hrpay.CountryCm,
		Amount:      5000,
		Description: "Commande #1024",
	})
	if err != nil {
		log.Printf("Failed to initiate cash-in: %v", err)
	} else {
		fmt.Printf("   ✅ Accepté — ref %s, statut %s\n", tx.Reference, tx.Status)
		fmt.Printf("      frais %.0f (%.1f%%), net %.0f\n", tx.Fee, tx.FeePercent, tx.NetAmount)

		// 3. Polling jusqu'au statut final
		fmt.Println("\n-> Polling transaction status...")
		final, err := client.Transactions.Poll(ctx, tx.Reference, hrpay.PollOptions{
			Interval: 5 * time.Second,
			Timeout:  60 * time.Second,
			OnStatus: func(status string, attempt int) {
				fmt.Printf("      tentative %d : %s\n", attempt, status)
			},
		})
		if err != nil {
			log.Printf("Polling failed: %v", err)
		} else {
			fmt.Printf("   ✅ Statut final : %s\n", final.Status)
		}
	}

	// 4. Validation locale — rejetée avant tout appel réseau
	fmt.Println("\n-> Testing client-side validation (montant sous le minimum de 100)...")
	_, err = client.CashIn.MobileMoney(ctx, hrpay.CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    hrpay.OperatorOrange,
		Amount:      50, // < 100
	})
	var valErr *hrpay.ValidationError
	if errors.As(err, &valErr) {
		fmt.Printf("   ✅ Validation locale : %v\n", valErr.Issues)
	} else {
		log.Printf("   ❌ Attendu une *ValidationError, obtenu : %v", err)
	}

	// 5. Erreurs métier typées
	fmt.Println("\n-> Cash-Out (peut échouer si le solde disponible est insuffisant)...")
	_, err = client.CashOut.MobileMoney(ctx, hrpay.CashOutMobileMoneyParams{
		PhoneNumber: "237680216505",
		Operator:    hrpay.OperatorMtn,
		Country:     hrpay.CountryCm,
		Amount:      1000000,
	})
	switch {
	case err == nil:
		fmt.Println("   ✅ Cash-Out accepté")
	case errors.Is(err, hrpay.ErrWalletBalanceInsufficient):
		var walletErr *hrpay.WalletError
		if errors.As(err, &walletErr) {
			fmt.Printf("   ✅ Solde insuffisant détecté (manque : %v)\n", walletErr.Details["shortfall"])
		}
	case errors.Is(err, hrpay.ErrKycNotApproved):
		fmt.Println("   ⚠️  KYC non approuvé — clés LIVE bloquées")
	default:
		log.Printf("   Erreur : %v", err)
	}

	fmt.Println("\n=== Example complete ===")
}
