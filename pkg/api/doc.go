// Package api is the parent namespace for generated FatSecret API service packages;
// each sub-package provides a typed Service interface and a New constructor.
//
// # Sub-packages
//
// The following namespaces are generated from the FatSecret REST API specification:
//
//   - exercise_entries — user exercise diary entries (oauth1_delegated)
//   - exercises — exercise reference data (client_credentials)
//   - feedback — user feedback submission (client_credentials)
//   - food — single-food operations: get, create, barcode lookup (mixed tiers)
//   - food_brands — brand catalog (client_credentials, premier scope)
//   - food_categories — food category catalog (client_credentials, premier scope)
//   - food_entries — user food diary entries (oauth1_delegated)
//   - food_sub_categories — food sub-category catalog (client_credentials, premier scope)
//   - foods — food search and favorites (mixed tiers)
//   - native — native app bridge endpoints (client_credentials)
//   - profile — user profile management (oauth1_signed / oauth1_delegated)
//   - recipe — single-recipe operations (client_credentials)
//   - recipe_types — recipe type catalog (client_credentials)
//   - recipes — recipe search (client_credentials)
//   - saved_meals — user saved meal templates (oauth1_delegated)
//   - weight — user weight entries (oauth1_delegated)
//
// # Service Pattern
//
// Each sub-package exposes a Service interface and a New constructor that
// accepts a [github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client.Client].
// Construct the low-level client once and pass it to as many service instances
// as needed:
//
//	c, err := client.NewClient(client.Options{Authenticator: myAuth})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer c.Close()
//
//	svc := foods.New(c)
//	result, err := svc.Search(ctx, foods.SearchRequest{
//	    SearchExpression: &expr,
//	})
//
// Generated code must not be edited directly. Regenerate with make generate.
package api
