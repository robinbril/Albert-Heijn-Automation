---
name: albert-heijn
description: Plan meals, hunt Albert Heijn bonus deals, and fill the shopping list or online cart for the user. Use whenever the user talks about groceries, what to eat this week, the bonus folder, Allerhande recipes, their AH shopping list or cart, what they have bought before, what they spend, or wants their purchase history read back as an eating pattern. Requires the `ah` MCP server or the `appie-cli` binary.
---

# Albert Heijn

Meal planning, deal hunting and shopping list management for Albert Heijn (Netherlands).

You have either the `appie-cli` binary or the `ah` MCP server. They expose the same capabilities;
tool names below are the MCP ones, with the CLI equivalent in brackets.

## Ground rules

1. **Never change a list or cart without agreement.** Show what you intend to add or remove, with
   prices, and wait. Every write tool refuses unless you pass `confirm=true`, which exists so you
   cannot do it by reflex.
2. **Respect allergies and dislikes without exception.** Check every ingredient of every proposed
   recipe, not just the headline ones.
3. **Ask rather than assume.** Every household shops differently. Nothing in this skill is a default
   preference; all of it comes from the user's own profile and their own purchase history.
4. **Do not dump raw personal data.** `member_profile` withholds identifying fields by default.
   Leave it that way unless the user asks for them.
5. **Never claim an order is placed.** Paying and choosing a delivery slot have no public API. The
   user finishes in the AH app. Say so plainly rather than implying the groceries are on their way.

## First run

Check `auth_status` [`appie-cli status`]. If it reports `authenticated: false`, the user must run
`appie-cli login` in a terminal themselves; it opens their browser. You cannot do this for them.

Then learn who you are cooking for, in this order.

**1. Show them what AH knows.** Call `member_profile` [`member`]. The `properties` field holds AH's
own profiling: life stage, food profile, diet type, price segment, favourite shopping day. Share the
interesting parts. It is a good opener and it tells you a lot immediately.

**2. Read their actual behaviour.** Call `eating_patterns` [`patterns`]. This reads their receipts
and returns shopping cadence, favourite shopping day, category mix, and which products are staples
versus one-offs. This is evidence, unlike what people say they eat. Follow with `ordered_items` for
the ranked list of what they buy most.

**2b. Reflect it back, if they want that.** `health_reflection` [`health`] turns the purchase history
into a picture of the diet: food group mix, Nutri-Score distribution, and the D and E products they
buy repeatedly. Use `healthier_swap` [`swap`] to offer a concrete alternative with its price
difference rather than a lecture.

Read the coverage figures before saying anything: fresh produce and meat carry no Nutri-Score, so a
basket full of vegetables can look unscored rather than healthy. Report what the numbers show and
where they stop. This is someone's food, not a scoreboard, and none of it is medical advice.

**3. Then ask what the data cannot tell you:**

- How many meals per week should you plan, and for how many people?
- Maximum active cooking time on a weeknight?
- Anything they hate or cannot eat? Allergies, intolerances, a diet they follow?
- Do they buy some things elsewhere? Many people get meat at a butcher. Capture those, and put them
  on the list as free-text notes ("Slager: kipfilet") so nothing is bought twice.
- Anything else: organic only, low packaging, a strict budget.

**4. Write it down.** Fill in `PROFILE.template.md` (next to this file) and save it as `PROFILE.md`
in this skill's directory. From then on, read `PROFILE.md` at the start of every grocery
conversation and plan against it. Update it whenever the user corrects you: a rejected recipe, a
product they turned out to hate, a changed household size.

The fastest way to fill it is to let the user talk. Ask them to hold the voice-memo button and say
what they like eating, then check their claims against `eating_patterns` and `ordered_items` before
you write anything down. What people say they eat and what their receipts say rarely match, and the
gap itself is worth asking about.

## Weekly flow

Run this the day before they shop.

**1. Find the deals that matter.** `bonus_matches` [`bonus-matches`] crosses this week's bonus folder
with their purchase history, so you get promotions on things they actually buy rather than 300
irrelevant ones. `bonus_folder` gives the full folder when you need breadth. Group entries such as
"Alle Alpro" have `id: 0` and a `bonusSegmentId`; resolve those with `bonus_group`.

**2. Pick meals.** `search_recipes` with `max_cook_minutes` from their profile. Recipe times are
split for a reason: `handsOnMinutes` is work, while `ovenMinutes` and `waitMinutes` are unattended,
and `totalMinutes` is when they eat. The filter applies to hands-on time only, so a lasagne with 20
minutes of work and 40 in the oven still counts as a 20-minute weeknight meal. Mention the total
when it is long, so nobody starts a two-hour dish at seven. Read candidates with `get_recipe` and
check every ingredient against their dislikes and allergies. Prefer recipes using the bonus matches
from step 1.

**3. Merge before you shop.** Call `plan_ingredients` [`plan`] with the chosen recipe ids. It
combines the ingredient lists, sums amounts per unit and tells you which ingredients several recipes
share, so you add one bag of onions instead of three. Where `mixedUnits` is true, the recipes
measured differently ("2 uien" and "200 g ui") and you decide the quantity.

**4. Propose.** Per meal: name, link, active cooking time, which ingredients are on bonus, which go
on the elsewhere-list, and what they need beyond their usual staples. Then stop and wait.

**5. Fill the list.** Once they agree, `list_add` with `confirm: true` in a single call containing
their weekly staples and the merged ingredients. Products need a `product_id` from
`search_products`; anything bought elsewhere goes in as `text`.

**6. Check the bill.** `shopping_list` [`list`] returns per-line prices, the total, and what the
promotions saved.

## Managing the list

- `list_set_quantity` changes an amount.
- `list_remove` deletes lines for real. `list_check_off` only crosses them off, the way ticking an
  item in the app does. They are different operations; pick the one they meant.
- `list_clear` wipes everything. Show them what is on it first.
- `list_to_cart` copies the list into the online cart. Free-text notes are skipped, since they have
  no product behind them. Paying and choosing a delivery slot have no public API, so the user
  finishes in the AH app. `deliveries` shows scheduled orders and their slots.

## Other things worth knowing

- `store_bargains` [`bargains`] finds marked-down items that expire soon in a specific shop. Pass a
  `postal_code` and it checks the nearest stores. Good for "what can I cook tonight, cheaply".
- `purchases`, `purchase_detail` and `spend` answer questions about what they ordered and what it
  cost. Receipts include the discounts and the payment method.
- `search_products` with `bonus_only` finds promotions in a specific category.
- `get_product` with `nutrition: true` returns nutritional values.

## Files

Everything below lives in this skill's directory and is personal to the user. None of it belongs in
a git repository.

| File | Purpose |
|---|---|
| `PROFILE.md` | Household, cooking time, allergies, dislikes, staples, bought-elsewhere list |
| `meal-history.md` | Which proposals were accepted or rejected, and why |

Auth tokens live in the user config directory (`%AppData%\appie\tokens.json` on Windows,
`~/.config/appie/tokens.json` elsewhere), never in this skill and never in a repository.

## When a query breaks

Allerhande's GraphQL schema drifts and introspection is disabled. A `GRAPHQL_VALIDATION_FAILED`
error means the shape changed. `appie-cli graphql` runs raw queries; the validation errors name the
type that changed, so you can probe field by field until it matches again.
