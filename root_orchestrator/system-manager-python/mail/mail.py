import os
import smtplib
from email.message import EmailMessage

gmail_user = os.environ.get("MAIL_USER", "")
gmail_password = os.environ.get("MAIL_PASSWORD", "")


class MailFactory:
    def __init__(self, user, payload=None):
        self.user = user
        self.payload = payload

    def send_mail(self):
        try:
            server = smtplib.SMTP_SSL("smtp.gmail.com", 465)
            server.ehlo()
            server.login(gmail_user, gmail_password)

            msg = self.create_message()
            msg["From"] = gmail_user
            msg["To"] = self.user["email"]
            print(self.user["email"])

            server.send_message(msg)
            server.quit()
            print("Email sent!")
        except Exception as e:
            print(e)
            print("Something went wrong while sending email...")

    def create_message(self) -> EmailMessage:
        pass


class RegistrationMailFactory(MailFactory):
    def create_message(self) -> EmailMessage:
        msg = EmailMessage()
        msg["Subject"] = "Oakestra-Project | Registration successful"
        content = (
            "Welcome to Oakestra-Team!\n"
            "You are successfully registered at Oakestra-Project!\n"
            "Your username: {username}\n"
            "Your password: {password}\n"
            "Your roles: {roles}"
        )
        rol = []
        for r in self.user["roles"]:
            rol.append(r["name"])
        content = content.format(
            username=self.user["name"], password=self.user["password"], roles=rol
        )
        msg.set_content(content)
        return msg


class UserUpdateMailFactory(MailFactory):
    def create_message(self) -> EmailMessage:
        msg = EmailMessage()
        msg["Subject"] = "Oakestra-Project | User account updated"
        content = (
            "Hi {username}!\n"
            "You account was updated by one of our admins.\n"
            "Your new data is: \n"
            "Your username: {username}\n"
            "Your roles: {roles}"
        )
        rol = []
        for r in self.user["roles"]:
            rol.append(r["name"])
        content = content.format(username=self.user["name"], roles=rol)
        msg.set_content(content)
        return msg


class ResetPasswordMailFactory(MailFactory):
    def create_message(self) -> EmailMessage:
        msg = EmailMessage()
        msg["Subject"] = "Oakestra-Project | Please reset your password"
        content = (
            "Hi {username}!\n"
            "We are sorry that you lost your password!\n"
            "To reset your password, press: {link}\n"
            "If you don't do this, this links expires within {expiry_delta} hours.\n"
        )
        expiry_delta = self.payload["expiry_delta"]
        expiry_delta_in_hours = expiry_delta.days // 24 + expiry_delta.seconds // 3600
        content = content.format(
            username=self.user["name"],
            link=self.payload["link"],
            expiry_delta=expiry_delta_in_hours,
        )
        msg.set_content(content)
        return msg
