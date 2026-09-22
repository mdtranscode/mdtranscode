# Markdown → DOCX Test

This has **bold**, *italic*, ***bold italic***, ~~strike~~, `inline code`, and a [link](https://example.com "title").

Hard break here.  
Next line after hard break.

## Lists

- Bullet one
  - Nested bullet with **bold**
    - Deeper bullet
- [x] Completed task
- [ ] Open task

5. Ordered starts at five
6. Second item
  1. Nested ordered
  2. Nested second
7. Back outer

> Blockquote paragraph with *emphasis*.
>
> - Quoted list item
> - Another item

## Table

| Left | Center | Right |
| :--- | :----: | ----: |
| one | **two** | 3 |
| `x \| y` | [link](https://openai.com) | ~~old~~ |

---

## Fenced code

```php
function hello($name) {
    echo "Hello, $name";
}
```

## Indented code

    SELECT id, name
    FROM users
    WHERE active = 1;

## HTML pre

<pre><code>line 1
    line 2 &lt;tag&gt;
line 3</code></pre>

## Image

![Tiny image](tiny.png)

Autolink: <https://example.org> and <person@example.org>.
