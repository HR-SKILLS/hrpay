# Changelog

## Unreleased — Mise en conformité Mobile Money + reconstruction Cartes Virtuelles

### ⚠️ Correctif de sécurité — rupture

- **Les appels de paiement/ressource envoyaient la mauvaise clé dans `Authorization`.** `createAuthMiddleware` posait la Clé A (publique) sur tous les appels — payin, payout, refund, cartes, etc. — au lieu de la Clé B (secrète), contrairement au contrat documenté de l'API. Corrigé : `Authorization: Bearer <Clé B>` sur ces appels, `X-Transaction-Token` inchangé. Aucune action requise côté intégrateur qui utilisait `WithAPIKeys(publicKey, secretKey)` normalement — mais si votre intégration contournait ce bug (ex. en inversant les arguments), corrigez l'ordre.
- **`GET /v1/wallet/balance`** utilise désormais le bon mode d'authentification (Clé B **seule**, sans `X-Transaction-Token`) et le bon chemin (`/v1/wallet/balance` au lieu de `/v1/balance`).

### Ajouté

- `WithIdempotencyKey(ctx, key)` — permet de fournir sa propre `Idempotency-Key` pour un retry client sûr après un timeout réseau, sur `CashIn.MobileMoney`, `CashOut.MobileMoney`, `Transactions.Refund`, et les endpoints cartes.
- `Transactions.Refund(ctx, reference)` — `POST /v1/payments/:reference/refund`.
- `Client.Fees` (`FeesService.List`) — `GET /v1/payments/fees`, barème de frais réel par pays/sens (ne jamais coder le taux 2 % par défaut en dur : des taux négociés existent par marchand).
- Couverture complète des 16 pays Mobile Money (ajout de `CountryCg`, `CountryTd`, `CountryCf`, `CountryNe`, `CountryGw`) et des opérateurs manquants (`OperatorWligdicash`, `OperatorCeltiis`), plus les maps de référence `MobileMoneyOperatorsByCountry` et `OtpRequiredOperators`.
- Champs `Reference` (optionnel) et `Country` (désormais requis, voir rupture ci-dessous) sur `CashInMobileMoneyParams`/`CashOutMobileMoneyParams`.
- `Transaction` (types.go) enrichi : `ExternalID`, `Country`, `FeeFixed`, `WalletBalanceBefore/After`, `Provider`, `ProviderRef`, `OtpRequired`, `CompletedAt`, `ErrorMessage`, `ErrorCode`, `FailureReasonCategory`.
- Nouveaux sentinels d'erreur Mobile Money : `ErrInvalidAmount`, `ErrInvalidOperatorCountry`, `ErrMissingPhone`, `ErrDuplicateReference`, `ErrWalletNotFound`, `ErrCountryNotActivated`, `ErrAmountExceedsLimit`, `ErrDailyLimitExceeded`, `ErrPlanTxLimitReached`, `ErrProviderUnavailable`, `ErrTooManyRequests`, `ErrCashoutRefused`.
- **Cartes Virtuelles — reconstruction quasi complète** :
  - `Client.Cards` est désormais une struct ombrelle `{Customers, Wallet, Virtual}`.
  - `card_customers.go` : KYC des porteurs (`CardCustomersService.Create/List/Get/Cards/Transactions/PollEnrollment`).
  - `card_wallet.go` : wallet USD dédié, cotation FX, tarification (`CardWalletService.Get/Quote/Fund/Withdraw/Pricing`).
  - `cards.go` réécrit : cycle de vie complet de la carte (`Create/List/Get/Topup/Withdraw/Freeze/Unfreeze/Terminate/Cancel/Transactions`), machine à états réaliste, gestion PAN/CVV via `reveal=true`.
  - Nouveaux sentinels d'erreur cartes (`ErrCustomerNotEnrolled`, `ErrCardNotActive`, `ErrMaxCardsReached`, etc.).
  - Nouvelles constantes d'événements webhook cartes (`card.created`, `card.funded`, `card.withdrawn`, `card.terminated`, `card.transaction.approved`, `card.transaction.declined`).

### Modifié — ruptures

- `CashIn.MobileMoney` et `CashOut.MobileMoney` retournent désormais `*Transaction` au lieu de `*CashInResponse`/`*CashOutResponse` (types supprimés). Le champ `Fee` devient `Fees` (le tag JSON `"fee"` était de toute façon incorrect avant : `"fees"`, qui ne correspondait à aucun champ réel de l'API). Le champ `Type` devient `Direction`.
- `CashInMobileMoneyParams.Country` et `CashOutMobileMoneyParams.Country` sont désormais **requis** — le SDK ne les défaute plus silencieusement vers `CountryCm`. La devise étant toujours dérivée du pays côté serveur, un pays incorrect ou absent est désormais rejeté côté client plutôt que d'envoyer une opération vers le mauvais pays.
- `Client.Cards` change de type : `*VirtualCardsService` → `*CardsService{Customers, Wallet, Virtual}`. Les anciens appels `client.Cards.Create(...)` deviennent `client.Cards.Virtual.Create(...)`.
- `WebhookEvent.Type` a désormais le tag JSON `"event"` (au lieu de `"type"`, qui ne correspondait à aucun champ réel de l'enveloppe webhook documentée). `WebhookEvent.MerchantID` ajouté.

### Supprimé

- `CashInService.Initiate` / `CashInInitiateParams` (`POST /v1/payments/initiate`) — route absente de la documentation actuelle, dupliquait `CashIn.MobileMoney` avec une validation plus faible.

### Corrigé

- `defaultCodeForStatus(422)` retourne désormais `VALIDATION_ERROR` (générique) au lieu de `OPERATOR_NOT_AVAILABLE` par défaut — un 422 sans code explicite couvre trop de cas distincts documentés pour deviner un sous-code précis.
