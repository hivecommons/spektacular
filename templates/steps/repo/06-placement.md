## Step {{step}}: {{title}}

Settle where this project keeps its own files for the repo. Most of the time this needs no question at all.

{{#writable}}
The default holds: those files go inside the repo being added. Take it silently. Do not describe it, do not offer it as a choice, and do not ask the user to approve it here. The confirmation step is where they see what will be created.

Raise it only if the conversation has already established that this repo should receive nothing but its own code. In that case, ask in their terms whether Spektacular may add a folder to their repo.
{{/writable}}
{{^writable}}
A folder cannot be created inside this repo, so the default is not available and you do have to ask. Ask in their terms whether Spektacular may add a folder to their repo, and if the answer is no, say that its files will live with the project instead and their repo will be left with only its code.

Do not explain the obstacle in Spektacular's terms and do not quote the error you would have hit.
{{/writable}}

Record `inside` when the files go in their repo and `project` when they do not. Those two words are for the command below and never for the user.

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Once the placement is settled, move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}","placement":"<inside or project>"}'
