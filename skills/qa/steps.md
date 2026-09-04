1. Open the app.
   tool: browser_open
   input: {"intent":"Open the app.","url":"{{arguments}}","expectation":"the report form is on the screen"}
   expect: the report form is on the screen

2. Say who is reporting.
   tool: browser_type
   input: {"intent":"Say who is reporting.","role":"textbox","name":"Your name","text":"Nerd Genie","expectation":"the name box holds what was typed"}
   expect: the name box holds what was typed

3. Say what happened.
   tool: browser_type
   input: {"intent":"Say what happened.","role":"textbox","name":"What happened","text":"The visual check walked the app and photographed every step.","expectation":"the box about what happened holds the words"}
   expect: the box about what happened holds the words

4. File the report.
   tool: browser_click
   input: {"intent":"File the report.","role":"button","name":"File the report","expectation":"the page says the report was filed"}
   expect: the page says the report was filed
