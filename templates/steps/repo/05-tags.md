## Step {{step}}: {{title}}

Agree a few tags that say what the repo is built with. These scope which work belongs to it, so a handful of accurate words is worth more than a long list.

Ask about exactly one thing here. This is the last of the four questions.

{{#evidence.readable}}
Here is what the repo says about itself:

{{#evidence.identity}}- it calls itself `{{evidence.identity}}`{{#evidence.manifest}}, in its `{{evidence.manifest}}`{{/evidence.manifest}}
{{/evidence.identity}}{{#evidence.summary}}- it describes itself as: {{evidence.summary}}
{{/evidence.summary}}{{#evidence.readme}}- its README opens: {{evidence.readme}}
{{/evidence.readme}}{{#evidence.languages_line}}- written in: {{evidence.languages_line}}
{{/evidence.languages_line}}{{#evidence.top_level_line}}- at its top level: {{evidence.top_level_line}}
{{/evidence.top_level_line}}

Propose tags drawn from this, chiefly from what it is written in and what it declares about itself, and state them inside the question so that agreeing alone is enough to record them. Offer a few, not a dozen, and let the user add or drop any of them.
{{/evidence.readable}}
{{^evidence.readable}}
This repo says nothing about itself that you can read, so you have nothing to propose. Ask the user which tags apply, and say plainly that you could not tell from the repo.
{{/evidence.readable}}

**If the user hands the rest over.** If at any point they say anything of the shape "just use what you think", stop asking. Draft the values they have not yet agreed, and carry every one of the four together in a single call to the placement step rather than asking again.

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Once the tags are agreed, move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}","tags":["<tag>","<tag>"]}'
