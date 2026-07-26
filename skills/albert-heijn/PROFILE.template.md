# Profile

Copy this file to `PROFILE.md` in the same directory and fill it in. The skill reads `PROFILE.md`
at the start of every grocery conversation and plans against it.

Two ways to fill it:

- **Let the agent do it.** Hold the voice-memo button in Claude Code and say what you like eating,
  what you never want to see again, how many people you cook for and how long you are willing to
  stand in the kitchen on a Tuesday. Then say: "check that against my purchase history and write my
  profile". The agent calls `eating_patterns` and `ordered_items`, compares them with what you said,
  and fills this in. It will ask about the differences, and those are usually the interesting parts.
- **Type it yourself.** Delete the italics, answer the questions, leave anything you do not care
  about empty. Empty means "no constraint", not "unknown".

Anything marked **hard rule** is never traded off, not for a bonus, not for a recipe that otherwise
fits. Everything else is a preference the agent may weigh.

---

## Household

- **People to cook for:** _2 / 4 / varies, say so_
- **Meals to plan per week:** _how many dinners you actually want planned, not how many you eat_
- **Shopping day:** _friday_
- **When you want the proposal:** _the evening before, so you can still change it_

## Time

- **Max hands-on cooking time, weekdays:** _30 minutes_
- **Max hands-on cooking time, weekends:** _no limit_

Hands-on means chopping and standing at the pan. Oven and rising time do not count, so a lasagne
with 20 minutes of work and 40 in the oven is a 20-minute weeknight meal.

## Hard rules

- **Allergies:** _lists everything, including traces if that matters_
- **Intolerances:** _lactose, gluten, whatever applies_
- **Diet:** _vegetarian, vegan, low sugar, halal, kosher, none_
- **Never buy:** _the things that end a recipe on sight_

Say what happens at the edges too: does "lactose-free" mean lactose-free dairy is fine, or no dairy
at all? Does "low sugar" mean no added sugar, or no fruit either? The agent will otherwise guess,
and it will guess wrong at the worst moment.

## Preferences

- **Cuisines you want more of:** _italian, thai, whatever_
- **Cuisines to skip:** _
- **Willing to repeat a meal:** _yes, once a fortnight is fine / no, never twice in a month_
- **Organic:** _only where it matters (name the products) / always / do not care_
- **Budget:** _a weekly ceiling, or leave empty_
- **Bonus:** _prefer bonus where it fits / chase it hard / ignore it_

## Staples

Things that go on the list every week without asking. The agent can propose this list from your
purchase history; check it before you accept it, because "bought often" and "wanted weekly" are not
the same thing.

- _
- _

## Bought elsewhere

Anything you do not buy at Albert Heijn: the butcher, the market, the Turkish shop, a subscription
box. These go on the list as free-text notes so you see them while shopping, and never as an AH
product, so nothing gets bought twice.

- _Butcher: chicken breast, minced beef_
- _

## Kitchen

- **Equipment you do not have:** _no air fryer, no food processor_
- **Things you refuse to do on a weeknight:** _deep frying, anything with more than one pan_

## Notes for the agent

Free text. Anything that does not fit above and that you would tell a person who shops for you.

_Example: I cook for two but one of us eats twice as much, so scale up the protein. Friday is
takeaway, never plan a meal for it. If something is on bonus and freezes well, buy two._
