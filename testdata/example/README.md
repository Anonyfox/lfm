# Reference page: immersive variant

The reference project is a single example, so this variant extends its documentation rather than
adding another scaffold. Use the design and implementation skills to build it from research.

For a physical workshop or an experience, keep a dark blue ground (#101d32) and white text. Give every
content section media: a first-screen photograph of the work, a loop showing the process, a drawing
explaining the joint and a photograph of the finished object. Add each to the concept's Imagery table
with its section, job, art direction and source. Cite real voice/peer IDs or facts for this choice.

```markdown
## Motion
Intensity: immersive
Moment: The first photograph settles into its final crop; a scroll scene separates the joint's layers; the process section loops the tool stroke while visible.
Feedback: The pause button changes its label between Pause process and Resume process.
Reduced motion: The first photograph shows its final crop; the joint shows the assembled end state; the process shows its poster and the adjacent explanation.
```

One process section can use this markup with the implementation skill's IntersectionObserver and
reduced-motion pattern. Encode AV1/WebM and H.264 MP4 without audio. Keep the poster visible with
reduced motion and wire the button to playback state, including the observer's condition.

```html
<section aria-labelledby="process">
  <h2 id="process">Cut along the grain</h2>
  <p>The blade follows the grain; stop before the shoulder and finish with a chisel.</p>
  <video muted loop playsinline autoplay preload="none" poster="process.webp" width="960" height="640">
    <source src="process.webm" type='video/webm; codecs="av01.0.05M.08"'>
    <source src="process.mp4" type="video/mp4">
  </video>
  <button type="button">Pause process</button>
</section>
```

The loop is below the first screen. Load only the first-screen sized photograph immediately; retain
posters and static frames for all motion fallbacks. These are media patterns, not new section types.
