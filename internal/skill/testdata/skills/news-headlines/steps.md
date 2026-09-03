1. Open the news site so the headlines are on the page.
   tool: browser_open
   input: {"url": "https://news.example.com/", "intent": "see the headlines"}
   expect: headlines

2. Read the page as a tree so the headlines can be listed.
   tool: browser_read
   input: {"url": "https://news.example.com/", "intent": "list the headlines"}
   expect: Rain

3. Post the summary, which cannot be taken back.
   tool: browser_click
   input: {"element": "post", "intent": "post the summary of {{arguments}}"}
