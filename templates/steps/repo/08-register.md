## Step {{step}}: {{title}}

Running the command below performs the registration. It is the first and only thing in this flow that writes anything.

You do not assemble it and you do not run a separate command to do the work. Everything gathered during the conversation is already recorded, and advancing past this step is what registers the repo.

If it fails, the flow stays where it is and nothing further has been written. Tell the user what went wrong in terms of their repo and their folder, not in terms of Spektacular's internals, and do not paste the raw error at them.

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}"}'
