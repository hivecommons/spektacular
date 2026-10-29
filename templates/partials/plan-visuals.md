{{! Included by the plan steps that draft a design section. Rendered with the step's own variables. }}
**Draw what a person has to see.** Some of what a plan describes is understood by looking, not by reading. When this section describes one of the two cases below, draw it as an SVG image and link it from the section, beside the prose rather than instead of it:

- **A visual change that one line of text cannot explain.** "Change the button to blue" needs no picture. A new screen, a rearranged layout, a new dialog, or any change to what a person sees whose shape matters does. Draw a design mock-up: the parts that change, labelled, with enough of their surroundings to place them.
- **A complex flow.** A flow with more than three steps, any branch or loop, or more than two parties handing work to one another. Draw a flow chart: one box per step, a diamond per decision, and one labelled arrow per transition.

Anything else stays as text: a picture that adds nothing is one more thing a reviewer has to open. Whether to draw is decided against these two rules, not put to the user.

Write every image as SVG by hand. A raster image or an HTML page is refused by the store. Give it a `viewBox`, text no smaller than 12px, dark strokes on a white background, and labels in the words the plan uses. Name it in lowercase words joined by hyphens, with no extension. Stage it under `.spektacular/tmp/{{plan_name}}/` with your own `Write` tool, then store it and remove the scratch file:

```
{{config.command}} plan image write {{plan_name}} <image-name> --from .spektacular/tmp/{{plan_name}}/<image-name>.svg
rm .spektacular/tmp/{{plan_name}}/<image-name>.svg
```

The command prints the image's `link`. Put exactly that link in this section's working file as `![<what the image shows>](<link>)`, so it resolves from `plan.md` in the plan store. Never write an image into the plan store with your own tools or build its link from a store path. `{{config.command}} plan image list {{plan_name}}` shows the images already stored; writing an existing name replaces it.
