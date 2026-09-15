## Step {{step}}: {{title}}

Establish which repo is being added: the folder its code lives in. This is the one thing you cannot work out for yourself, which is why it is the only question asked empty-handed. Every question after it arrives with an answer already drafted, so the user is mostly agreeing with you rather than filling in blanks.

{{#repo_path}}
The user already named the repo, and it is `{{repo_path}}`. Do not ask for it again. Move straight on.
{{/repo_path}}
{{^repo_path}}
Ask which repo they want to add, in terms of the folder its code is in. Ask for nothing else yet, and do not raise its name, what it is, or where anything will be written.

Resolve their answer to an absolute path and check the folder exists. If it does not, say so plainly and ask again. Never quietly substitute a directory that looks close to what they said.
{{/repo_path}}

**What is yours and what is theirs.** Everything about how this gets recorded is working detail for you. So is the vocabulary: *footprint*, *source*, *provider*, *colocated* and *separate* used as terms of art, the names of the files Spektacular writes, and the name, arguments and flags of every command you run. Those words name Spektacular's internals, and a user adding their repo did not ask to learn them. Speak to them about their repo instead: what it is called, where its code lives, and what folder will be created. Never narrate the mechanics, never justify a choice by what a command reported back, and never explain a step by describing a file Spektacular is about to write.

Once you have the folder, move to the next step by running the command:

{{config.command}} repo goto --data '{"step":"{{next_step}}","location":"<the folder the repo's code is in>"}'
