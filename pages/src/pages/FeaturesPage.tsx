import React from 'react';
import HeroSection from '../components/HeroSection';
import HighlightsSection from '../components/HighlightsSection';
import UseCasesSection from '../components/UseCasesSection';
import FeaturesSection from '../components/FeaturesSection';
import BenchmarkSection from '../components/BenchmarkSection';
import QuickStartSection from '../components/QuickStartSection';
import Footer from '../components/Footer';
import FadeInSection from '../components/FadeInSection';

const FeaturesPage: React.FC = () => {
  return (
    <>
      <FadeInSection>
        <HeroSection />
      </FadeInSection>
      <FadeInSection delay={100}>
        <HighlightsSection />
      </FadeInSection>
      <FadeInSection delay={100}>
        <UseCasesSection />
      </FadeInSection>
      <FadeInSection delay={100}>
        <FeaturesSection />
      </FadeInSection>
      <FadeInSection delay={100}>
        <BenchmarkSection />
      </FadeInSection>
      <FadeInSection delay={100}>
        <QuickStartSection />
      </FadeInSection>
      <FadeInSection>
        <Footer />
      </FadeInSection>
    </>
  );
};

export default FeaturesPage;
