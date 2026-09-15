## Step {{step}}: {{title}}

Agree the name this project will know the repo by.

This step and the three that follow it use the Flipped Interaction pattern (White et al., "A Prompt Pattern Catalog to Enhance Prompt Engineering with ChatGPT," arXiv:2302.11382): rather than asking the user to supply facts their own code already states, you read the repo, propose an answer, and let them agree or correct it.

Ask about exactly one thing here. Do not mention the description, the role or the tags in the same message; each gets its own turn.

{{#evidence.readable}}
Here is what the repo says about itself:

{{#evidence.identity}}- it calls itself `{{evidence.identity}}`{{#evidence.manifest}}, in its `{{evidence.manifest}}`{{/evidence.manifest}}
{{/evidence.identity}}{{#evidence.summary}}- it describes itself as: {{evidence.summary}}
{{/evidence.summary}}{{#evidence.readme}}- its README opens: {{evidence.readme}}
{{/evidence.readme}}{{#evidence.languages_line}}- written in: {{evidence.languages_line}}
{{/evidence.languages_line}}{{#evidence.top_level_line}}- at its top level: {{evidence.top_level_line}}
{{/evidence.top_level_line}}

Propose a name drawn from this, not from the folder name, and state it inside the question so that agreeing alone is enough to record it. Prefer what the repo calls itself where that reads well. It has to be unique in this project and safe to use in a path.

**Offer to take the rest, once.** Along with this first proposal, tell the user you can fill in the remaining details yourself if they would rather not go through them one at a time. Make the offer here and nowhere else.
{{/evidence.readable}}
{{^evidence.readable}}
This repo says nothing about itself that you can read, so you have nothing to propose. Ask the user what it should be called, and say plainly that you could not tell from the repo. Do not guess from the folder name and present it as though you had read it.
{{/evidence.readable}}

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Once the name is agreed, move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}","name":"<the agreed name>"}'

**If they hand the rest over.** If they accept the offer, or say anything of the shape "just use what you think", do not ask the remaining questions at all. Draft the description, role and tags yourself and carry all four values together in one call, which skips those questions entirely:

{{config.command}} repo goto --data '{"step":"placement","name":"<name>","description":"<description>","role":"<role>","tags":["<tag>"]}'
