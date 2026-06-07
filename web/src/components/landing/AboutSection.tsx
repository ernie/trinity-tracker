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
          Trinity is a community for people who know the best arena shooter ever
          made was released in 1999. Some of us never stopped playing. Some are
          coming home. Some are showing up for the first time, finding out what
          a shooter feels like when it&rsquo;s just movement, weapons, and your
          opponents.
        </p>
        <p>
          Quake III had one more move left in it: VR. You&rsquo;re{" "}
          <em>inside</em> the game now &mdash; real aim, real space between you
          and your opponent &mdash; and flatscreen and VR share every match. If
          you played the original, it asks far more of you than you remember.
        </p>
        <p>
          This site is the tracker. Watch fights live, or replay them &mdash;
          every match is recorded. Bring your own copy of Quake III to play;
          watching needs nothing at all.
        </p>
      </div>
    </section>
  );
}
