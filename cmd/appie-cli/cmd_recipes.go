package main

import (
	"context"
	"flag"

	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

func recipeCommands() []command {
	var (
		limit       int
		maxCookTime int
	)

	return []command{
		{
			name:    "recipes",
			args:    "[query]",
			summary: "Search Allerhande recipes",
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "limit", 10, "maximum results")
				fs.IntVar(&maxCookTime, "max-time", 0, "max hands-on minutes; oven and waiting time do not count")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				recipes, err := c.SearchRecipes(ctx, ah.RecipeSearchOptions{
					Query:       arg(fs, 0),
					Limit:       limit,
					MaxCookTime: maxCookTime,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"count": len(recipes), "recipes": recipes}, nil
			},
		},
		{
			name:    "recipe",
			args:    "<id>",
			summary: "Read a recipe: ingredients, steps and nutrition",
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id, err := intArg(fs, 0, "id")
				if err != nil {
					return nil, err
				}
				return c.GetRecipe(ctx, id)
			},
		},
		{
			name:    "plan",
			args:    "<recipe-id> [recipe-id...]",
			summary: "Merge several recipes into one deduplicated ingredient list",
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				ids, err := intArgs(fs, "recipe-id")
				if err != nil {
					return nil, err
				}

				recipes := make([]ah.Recipe, 0, len(ids))
				for _, id := range ids {
					recipe, err := c.GetRecipe(ctx, id)
					if err != nil {
						return nil, err
					}
					recipes = append(recipes, *recipe)
				}

				merged := ah.MergeIngredients(recipes)
				titles := make([]string, 0, len(recipes))
				shared := 0
				for _, r := range recipes {
					titles = append(titles, r.Title)
				}
				for _, ing := range merged {
					if len(ing.Recipes) > 1 {
						shared++
					}
				}

				return map[string]any{
					"recipes":         titles,
					"ingredientCount": len(merged),
					"sharedCount":     shared,
					"ingredients":     merged,
				}, nil
			},
		},
	}
}
