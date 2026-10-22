> **Note:** This is the original pitch and is kept for its narrative. Where it differs from the current design, `GLOSSARY.md`, `docs/adr/` and the GitHub issues take precedence. Notable differences: planning is per Cycle (~10 days between deliveries) rather than per week; there is no separate Freezer Stash (Portions and Stock Items have a Storage Location); replanning happens through approved Plan Proposals; the guided profiler and Weight Check-ins come in v2; Cooking Mode is v3; all food is prepared in Prep Sessions that yield Portions, including breakfast, snacks and Ingredients eaten as they are; and planning is grounded in a locally stored Known Catalog.

# Product Vision & Architectural Concept

MealOps is an intelligent, adaptive nutritional assistant and agentic co-pilot designed for individuals who want to eat healthy, structured meals without turning meal planning and cooking into a second full-time job. Unlike traditional meal planning applications — which are rigid, static, and focused purely on theoretical planning — MealOps acts as a real-time co-pilot for your kitchen, capable of orchestrating grocery delivery, tracking pantry and freezer stocks, and dynamically reacting to everyday schedule changes.

## Core Philosophy & Operational Workflow

1. Interactive Onboarding & Goal Profiling
For users who do not know their exact caloric or macronutrient requirements, MealOps begins with a guided, conversational onboarding profiler. By asking targeted questions regarding body metrics, lifestyle, daily movement, and health goals, the system calculates a personalized baseline for daily calories and macronutrient splits (protein, carbs, fats).

2. Low-Care Cooking & Asymmetric Batching ("Cook 1, Eat 3")
Recognizing that weekday lunch breaks offer far less time than evening dinners, MealOps shifts the heavy lifting to dinner sessions while keeping active kitchen time to a minimum:

- "Low-Care" Appliance-First Recipes: The engine prioritizes recipes utilizing appliances that require minimal active supervision — such as Microwave Ovens, Electric Ovens, Instant Pots, Air Fryers, and Slow Cookers. Prep work is capped at 10–15 minutes, letting appliances handle the rest passively.

- Asymmetric Portion Allocation: When cooking dinner, the system defaults to generating 3 portions:

    * Portion 1: Consumed immediately for dinner.

    * Portion 2: Scheduled as a quick lunch 2 days later (staggered to avoid back-to-back meal repetition).

    * Portion 3: Tagged and added to the Freezer Stash for zero-prep retrieval on busy days.

3. Real-Time Grocery E-Commerce & Playwright MCP Cart Automation
To bridge the gap between meal plans and store availability, MealOps integrates directly with online grocery stores (e.g., local supermarket delivery platforms). Driven by a throttled, rate-limited Playwright MCP (Model Context Protocol) Server, the agent safely browses live store catalogs, verifies actual product availability, unit prices, and package sizes, and automatically populates your online shopping cart — grounding every generated recipe in items you can actually buy and deliver.

4. Dynamic Event Recalibration
When real-life interruptions occur — a last-minute dinner out, an unexpected late meeting, or an unconsumed leftover — the user simply inputs the event. The AI agent instantly recalculates the remainder of the week's meal schedule, reallocating existing pantry and freezer items to guarantee zero food waste while maintaining target weekly nutritional macros.

5. Hands-Free Kitchen "Cooking Mode"
When it is time to cook, MealOps transitions into a distraction-free interface optimized for smartphones and tablets resting on kitchen counters:

Screen Keep-Alive: Uses the HTML5 Web Screen Wake Lock API to keep the screen active continuously, eliminating the need to unlock the device with wet or messy hands.

High-Contrast Layout: Features large typography, exact session-adjusted ingredient quantities, step-by-step instructions, and integrated hands-free timers.

## Complete Key Feature List

- Guided Macro & Calorie Profiler: Interactive questionnaire estimating daily caloric needs and macronutrient distribution targets based on individual physical metrics and goals.

- "Cook 1, Eat 3" Asymmetric Batching Engine: Automated scheduling that yields an immediate dinner portion, a deferred lunch portion (staggered 2+ days to prevent monotony), and a frozen portion.

- Low-Care Appliance Optimization: Priority filtering for recipes optimized for Microwave, Electric Oven, Instant Pot, and Air Fryer, minimizing active stovetop management.

- Pantry & Freezer Stash Inventory Tracker: Digital tracking for fresh pantry items, expiration dates, and frozen prepared portions ready for rapid microwave or oven reheating.

- Playwright MCP Grocery Cart Automation: Rate-limited local browser agent that searches live supermarket catalogs, verifies stock, and auto-fills online delivery carts with exact recipe ingredients.

- Dynamic Event Recalibration Engine: Real-time schedule realignment triggered by last-minute events (eating out, schedule changes, leftover variations) with zero-waste optimization.

- Smart Consolidated Grocery List: Aggregated shopping list categorized by supermarket aisle, cross-referenced with existing pantry inventory to avoid duplicate purchases.

- Hands-Free Cooking Mode with Screen Keep-Alive: Full-screen kitchen display utilizing the Web Screen Wake Lock API to lock the display on, featuring step-by-step instructions, session portioning, and active timers.

- One-Tap Swap & Substitute: Fast substitution of individual ingredients or full meals with nutritionally equivalent alternatives based on live supermarket availability.

- Semantic Vector Recipe Search: Natural language search powered by PostgreSQL pgvector, filtering recipes by available pantry/freezer ingredients, low-care appliance type, prep time, or current cravings.

- Agent Execution Trace Panel (Portfolio Showcase): Developer-centric interface displaying the AI agent's step-by-step chain-of-thought, Playwright interactions, and MCP tool invocations in real time.
