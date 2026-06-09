# FatSecret Integration — Deferred

The FatSecret integration (barcode scanning) is deferred to post-MVP.

## Current state

The `fatsecret-apiclient-go` client library is complete and lives in this repository. It is not imported by any backend command handler, agent, or translation path. No FatSecret API calls are made at runtime.

## MVP food data source

The MVP uses **USDA FoodData Central** for all food lookups. USDA covers the text-search path (the primary food logging modality) and provides leucine, protein, fat, carbohydrates, and calories per serving. No API key or Business Associate Agreement is required — USDA FoodData Central is a public federal database.

The relevant backend path:

- Agent: `food-query-responder` (NATS request-reply on `peppi.query.translation.usda-search`)

- Translation: `apps/backend/internal/commandhandler/foodlog/enter_food_log.go` → `translateUSDA()`

- Database: local SQLite snapshot, path via `USDA_DB_PATH` env var, synced by `usda-sync` agent

## What remains to wire when FatSecret is resumed

1. **Barcode lookup** — call `foods.GetByBarcode(ctx, barcode)` (or the equivalent FatSecret `food.find_id_for_barcode` method) using the OAuth 2.0 client-credentials authenticator already implemented in this library.

2. **Translation** — write a `translateFatSecret(f fatsecret.Food) MealItem` function in `apps/backend/internal/commandhandler/foodlog/` parallel to `translateUSDA()`. Map FatSecret's `ServingsServing` fields to `MealItem.ProteinG`, `FatG`, `CarbG`, `Calories`, and `LeucineMg` (leucine is not in the FatSecret response — source it from the USDA leucine table by food name match or accept zero with a known gap).

3. **Input mode** — the `EnterFoodLog` command has an `input_mode` field. Add `"barcode"` as a recognised value and route barcode entries through the FatSecret translation path.

4. **Client wiring** — register a `FatSecretSearcher` (or equivalent interface) in `apps/backend/cmd/peppid/agent/command_dispatcher.go` alongside the existing `cdUSDASearcher`, `nxGated`, and `edamamGated` registrations.

5. **Credentials** — `FATSECRET_CLIENT_ID` and `FATSECRET_CLIENT_SECRET` environment variables must be set. FatSecret Premier Free has been approved; confirm the account tier supports barcode lookup before enabling.

6. **BAA** — FatSecret barcode lookups do not inherently include PHI (the barcode is a product identifier, not a patient identifier). Confirm with legal whether a BAA is required given how patient context is attached to the request downstream.

## Timeline

Post-MVP / future milestone. No delivery date is set. Resume after USDA text-search and macro mapping are complete and verified in production.

---

*Last updated: 2026-06-09*
