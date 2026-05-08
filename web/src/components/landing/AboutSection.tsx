export function AboutSection() {
  return (
    <section className="landing-section landing-about">
      <div className="landing-about__ornament" aria-hidden>
        <span className="landing-about__rule" />
        <span>✦</span>
        <span className="landing-about__rule" />
      </div>
      <div className="landing-about__prose">
        <p>
          Trinity is a community for people who know the best arena shooter ever made was released in 1999.
          Some of us never stopped playing. Some are coming home. Some are showing up for the first time,
          finding out what a shooter feels like when it&rsquo;s just movement, weapons, and your opponents.
        </p>
        <p>
          Trinity exists because Quake III had one more move to make: VR. Same maps, same weapons, same
          fights &mdash; but now you&rsquo;re <em>inside</em> the game: real aim, and real space between you
          and your opponent. Flatscreen and VR share every match. If you played the original, this is far
          better than you remember &mdash; and requires <em>much</em> more from you as a player.
        </p>
        <p>
          This site is the tracker. Every match recorded, every demo replayable in your browser. Bring your
          own copy of Quake III to play; spectate without one.
        </p>
      </div>
    </section>
  )
}
