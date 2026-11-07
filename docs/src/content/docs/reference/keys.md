---
title: Keys
description: Every key binding, split by the context it works in.
---

pile has four contexts, each with its own keys: the board, the search prompt,
the detail overlay, and the help page. Actions you configure add their own keys
to the board and the help page.

## Board

| key | action |
| --- | --- |
| `j` / `k` | move down / up (also `down` / `up`) |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `pgdn` / `pgup` | a full page down / up |
| `l` / `h` | next / previous section (also `right` / `left`) |
| `g` / `G` | top / bottom (also `home` / `end`) |
| `enter` / `o` | open the selected PR in the browser |
| `d` | open the detail overlay for the selected PR |
| `y` | copy the selected PR's URL |
| `/` | open the search prompt |
| `n` / `N` | next / previous match, wrapping |
| `r` | refresh the board |
| `?` | open the help page |
| `q` / `esc` / `ctrl+c` | quit |

`esc` clears the search highlights when a query is live, and quits otherwise.

## Search prompt

While the prompt is open, every printable key belongs to the query, and
navigation moves to chords.

| key | action |
| --- | --- |
| text | type into the query |
| `backspace` | delete the last character |
| `ctrl+n` / `ctrl+j` / `down` | next match while typing |
| `ctrl+p` / `ctrl+k` / `up` | previous match while typing |
| `enter` | keep the query and its highlights |
| `esc` | cancel the search |
| `ctrl+c` | quit |

## Detail overlay

The overlay is a look, not a mode: `d` opens it and it closes on the next
movement key.

| key | action |
| --- | --- |
| `j` / `k` / `l` / `h` | close and move (also the arrow keys) |
| `g` / `G` | close and go to top / bottom |
| any other key | close |
| `ctrl+c` | quit |

## Help page

The help page scrolls and searches; the bottom row of the page always lists how
to leave it.

| key | action |
| --- | --- |
| `j` / `k` | scroll down / up (also `down` / `up`) |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `pgdn` / `pgup` | a full page down / up |
| `g` / `G` | top / bottom (also `home` / `end`) |
| `/` | search within the page |
| `n` / `N` | next / previous match |
| `esc` / `q` / `?` | close the page |
| any other key | close the page |
| `ctrl+c` | quit |

The help page's search prompt is the board's, with the cursor swapped for the
scroll position.
