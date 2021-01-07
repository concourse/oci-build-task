FROM scratch AS additional-target
LABEL target=additional-target
USER banana
ADD Dockerfile /Dockerfile.banana
ENV PATH=/darkness
ENV BA=nana

FROM scratch AS final-target
LABEL target=final-target
USER orange
ADD Dockerfile /Dockerfile.orange
ENV PATH=/lightness
ENV OR=ange
