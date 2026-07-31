import React from 'react';
import FeaturesSection from '../components/FeaturesSection';
import Footer from '../components/Footer';
import FadeInSection from '../components/FadeInSection';

const FeaturesRoutePage: React.FC = () => {
  return (
    <div style={{ paddingTop: 72 }}>
      <FadeInSection>
        <FeaturesSection />
      </FadeInSection>
      <FadeInSection>
        <Footer />
      </FadeInSection>
    </div>
  );
};

export default FeaturesRoutePage;
