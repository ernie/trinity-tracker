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
          made came out in 1999.
        </p>
        <p>
          It&rsquo;s Quake III Arena the way you remember it, which is better
          than it was. Widescreen works, the interface is sharp at any
          resolution, the renderer looks better, the settings menus have the
          options that matter, and voice chat is built in. VR is fully
          supported, and flatscreen and VR players share the same matches.
        </p>
        <p>
          Long-standing bugs are fixed, especially in Team Arena. The grappling
          hook id Software never finished has been reimagined with new art and
          sound. Blood and damage effects are better, and small quality-of-life
          improvements run through the menus and the game.
        </p>
        <p>
          This site is the tracker. Every match is recorded, so you can watch
          fights live or replay them later. Playing needs your own copy of Quake
          III Arena; watching needs nothing.
        </p>
      </div>
    </section>
  );
}
