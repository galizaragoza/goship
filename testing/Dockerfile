FROM alpine:latest

RUN apk add --no-cache openssh rsync\
  && ssh-keygen -A \
  && mkdir -p /root/.ssh /test/testdir \
  && chmod 700 /root/.ssh \
  && sed -i 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config \
  && sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/'  /etc/ssh/sshd_config \
  && sed -i 's/^#*StrictModes.*/StrictModes no/'                        /etc/ssh/sshd_config

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
