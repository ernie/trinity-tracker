import { Link } from 'react-router-dom'

// /features — top-level marketing/showcase route. Lifts the
// video-bearing feature cards from the old DocsFeatures page;
// future phases will trim, restructure, and add platform-aware
// variants. For now the videos and basic descriptions are enough
// to ship the new entry point.
export function FeaturesPage() {
  return (
    <div className="features-page">
      <header className="features-page__hero">
        <h1 className="features-page__title">What's in Trinity?</h1>
        <p className="features-page__lead">
          Trinity layers modern conveniences on top of Quake 3 — VR
          support, voice chat, demo recording and playback, and more.
          It's <strong>backwards-compatible with vanilla Quake 3
          servers</strong>, so you can install it and still play
          anywhere.
        </p>
        <div className="features-page__cta-row">
          <Link to="/docs/getting-started" className="features-page__cta features-page__cta--primary">
            Install Trinity
          </Link>
          <Link to="/docs" className="features-page__cta">
            Read the docs
          </Link>
        </div>
      </header>

      <section className="features-page__grid">
        <FeatureCard
          name="VR Tracking"
          desc="1:1 head and weapon hand tracking — your player's head and weapon move exactly as you do."
          videoBase="vr_tracking"
        />
        <FeatureCard
          name="Voice Chat"
          desc="Opus-based voice chat on supported servers, also played back in TV demos. Push-to-talk or voice-activity, with spatial / team / direct channels."
          videoBase={null}
          cvar="cl_voip 1"
        />
        <FeatureCard
          name="Damage Plums"
          desc="Floating damage numbers appear on each hit, showing exactly how much damage you dealt."
          videoBase="cg_damagePlums"
          cvar="cg_damagePlums 1"
        />
        <FeatureCard
          name="Blood Particles"
          desc="Particle-based blood effects with wall and floor splats, replacing the default sprite blood."
          videoBase="cg_bloodParticles"
          cvar="cg_bloodParticles 1"
        />
        <FeatureCard
          name="Damage Effect"
          desc="Directional red vignette when taking damage, replacing the default blood splatter overlay."
          videoBase="cg_damageEffect"
          cvar="cg_damageEffect 1"
        />
        <FeatureCard
          name="Orbit Camera"
          desc="Third-person orbit camera for spectating."
          videoBase="orbit_camera"
          cvar="cg_followMode 1 / cg_smoothFollow 1"
        />
        <FeatureCard
          name="TV Demo Scrubbing"
          desc="Scrub forward and backward through recorded TV demos."
          videoBase="tvd_scrub"
          cvar="+tv_scrub"
        />
        <FeatureCard
          name="TV Demo Pause"
          desc="Pause and resume TV demo playback."
          videoBase="tvd_pause"
          cvar="demopause"
        />
        <FeatureCard
          name="Stencil Shadows"
          desc="Shadow volumes with BSP clipping to prevent wall and floor bleed-through."
          videoBase={null}
          cvar="r_shadows 2"
        />
      </section>
    </div>
  )
}

interface FeatureCardProps {
  name: string
  desc: string
  videoBase: string | null
  cvar?: string
}

function FeatureCard({ name, desc, videoBase, cvar }: FeatureCardProps) {
  return (
    <article className="features-page__card">
      <header className="features-page__card-header">
        <h2 className="features-page__card-title">{name}</h2>
        {cvar && <code className="features-page__card-cvar">{cvar}</code>}
      </header>
      <p className="features-page__card-desc">{desc}</p>
      {videoBase && (
        <video autoPlay loop muted playsInline className="features-page__card-video">
          <source src={`/assets/videos/${videoBase}.webm`} type="video/webm" />
          <source src={`/assets/videos/${videoBase}.mp4`} type="video/mp4" />
        </video>
      )}
    </article>
  )
}
