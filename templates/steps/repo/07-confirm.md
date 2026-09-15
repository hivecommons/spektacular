## Step {{step}}: {{title}}

Nothing has been written yet. Before anything is, tell the user plainly what is about to happen and wait for them to agree.

Say three things, in their terms:

- which repo is being registered, and what it will be known as
- which folder will be created, by its full path
- where its code lives

Two or three sentences is right. Something of the shape: "I'll register `{{repo_name}}` and create `{{repo_path}}/.spektacular/`. Nothing else in that repo is touched. Shall I go ahead?"

Then stop and wait. A confirmation has to be explicit. Silence, a change of subject, or an ambiguous reply is not agreement, and neither is the absence of an objection.

**Do not put any command in front of them.** Not its name, not its arguments, not its flags, and not the contents of any file that is about to be written. If they ask what exactly will be created, answer with the folder and what it is for, in their terms.

**If they decline,** nothing is written and nothing needs undoing. Stay here, find out what they would rather do, and change the answer they object to before coming back.

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Once the user has explicitly agreed, move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}"}'
